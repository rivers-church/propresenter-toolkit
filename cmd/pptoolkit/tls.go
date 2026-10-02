package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/libdns/cloudflare"
)

// tlsSettings come from the environment (see deploy/install.sh).
type tlsSettings struct {
	Domain    string // e.g. protoolkit.rivers.church
	Token     string // Cloudflare API token: Zone:Read + DNS:Edit on the zone
	Email     string // optional, for Let's Encrypt expiry notices
	CertDir   string // where certificates and the ACME account are kept
	HTTPSAddr string // default ":443"
	Staging   bool   // use Let's Encrypt's staging CA (for testing)
}

func tlsFromEnv(stylesDir string) (*tlsSettings, error) {
	domain := strings.TrimSpace(os.Getenv("PPT_DOMAIN"))
	if domain == "" {
		return nil, nil
	}
	s := &tlsSettings{
		Domain:    domain,
		Token:     strings.TrimSpace(os.Getenv("PPT_CLOUDFLARE_API_TOKEN")),
		Email:     strings.TrimSpace(os.Getenv("PPT_ACME_EMAIL")),
		CertDir:   envOr("PPT_CERT_DIR", filepath.Join(filepath.Dir(filepath.Clean(stylesDir)), "certs")),
		HTTPSAddr: envOr("PPT_HTTPS_ADDR", ":443"),
		Staging:   os.Getenv("PPT_ACME_STAGING") == "1",
	}
	if s.Token == "" {
		return nil, errors.New("PPT_DOMAIN is set but PPT_CLOUDFLARE_API_TOKEN is empty - HTTPS needs a Cloudflare API token to prove the domain is yours")
	}
	return s, nil
}

// serveTLS gets (and keeps renewing) a Let's Encrypt certificate for the
// domain using a Cloudflare DNS challenge, then serves the app over HTTPS
// and redirects plain HTTP to it.
//
// The DNS challenge works for a site that's only reachable inside the
// network: Let's Encrypt never connects to the server, it just checks a
// temporary TXT record the app creates in Cloudflare.
func serveTLS(s *tlsSettings, httpAddr string, app http.Handler) error {
	certmagic.Default.Storage = &certmagic.FileStorage{Path: s.CertDir}
	certmagic.DefaultACME.Agreed = true
	certmagic.DefaultACME.Email = s.Email
	if s.Staging {
		certmagic.DefaultACME.CA = certmagic.LetsEncryptStagingCA
	}
	certmagic.DefaultACME.DisableHTTPChallenge = true
	certmagic.DefaultACME.DisableTLSALPNChallenge = true
	certmagic.DefaultACME.DNS01Solver = &certmagic.DNS01Solver{
		DNSManager: certmagic.DNSManager{
			DNSProvider: &cloudflare.Provider{APIToken: s.Token},
			// Internal DNS servers often host their own zone for this name
			// (to point it at a LAN address) and can't see the challenge
			// record, so check propagation against public resolvers.
			Resolvers:          []string{"1.1.1.1:53", "8.8.8.8:53"},
			PropagationTimeout: 5 * time.Minute,
		},
	}

	cfg := certmagic.NewDefault()
	log.Printf("Getting an HTTPS certificate for %s (this can take a minute the first time)...", s.Domain)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := cfg.ManageSync(ctx, []string{s.Domain}); err != nil {
		return fmt.Errorf("getting a certificate for %s: %w (check the Cloudflare token can edit DNS for the zone)", s.Domain, err)
	}
	log.Printf("Certificate ready; it renews automatically.")

	tlsCfg := cfg.TLSConfig()
	tlsCfg.NextProtos = append([]string{"h2", "http/1.1"}, tlsCfg.NextProtos...)
	tlsCfg.MinVersion = tls.VersionTLS12

	errc := make(chan error, 2)
	go func() {
		redirect := &http.Server{Addr: httpAddr, Handler: redirectToHTTPS(s.Domain, s.HTTPSAddr), ReadHeaderTimeout: 10 * time.Second}
		log.Printf("Redirecting http on %s to https://%s", httpAddr, s.Domain)
		errc <- redirect.ListenAndServe()
	}()
	go func() {
		ln, err := tls.Listen("tcp", s.HTTPSAddr, tlsCfg)
		if err != nil {
			errc <- err
			return
		}
		log.Printf("Serving https://%s on %s", s.Domain, s.HTTPSAddr)
		hs := &http.Server{Handler: app, ReadHeaderTimeout: 10 * time.Second}
		errc <- hs.Serve(ln)
	}()
	return <-errc
}

// redirectToHTTPS sends every plain-HTTP request to the same path on the
// HTTPS site.
func redirectToHTTPS(domain, httpsAddr string) http.Handler {
	host := domain
	if _, port, err := net.SplitHostPort(httpsAddr); err == nil && port != "" && port != "443" {
		host = net.JoinHostPort(domain, port)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+host+r.URL.RequestURI(), http.StatusPermanentRedirect)
	})
}
