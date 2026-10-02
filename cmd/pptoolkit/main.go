// Command pptoolkit serves the ProPresenter Toolkit web app, which turns
// sermon-notes PDFs into ProPresenter 7 .pro files.
//
// Configuration comes from flags or environment variables:
//
//	-addr    PPT_ADDR        listen address          (default ":5000")
//	-styles  PPT_STYLES_DIR  style profiles folder   (default "styles")
//	-auth    PPT_AUTH        "user:password" login   (default: none)
//
// For HTTPS with an automatic Let's Encrypt certificate, also set
// PPT_DOMAIN and PPT_CLOUDFLARE_API_TOKEN (see tls.go); -addr then serves
// a redirect to https.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/rivers-church/propresenter-toolkit/internal/style"
	"github.com/rivers-church/propresenter-toolkit/internal/web"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	log.SetFlags(0)
	if err := run(os.Args[1:]); err != nil {
		log.Fatal("error: ", err)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("pptoolkit", flag.ExitOnError)
	addr := fs.String("addr", envOr("PPT_ADDR", ":5000"), "address to listen on (env PPT_ADDR)")
	stylesDir := fs.String("styles", envOr("PPT_STYLES_DIR", "styles"), "folder holding style profiles (env PPT_STYLES_DIR)")
	auth := fs.String("auth", os.Getenv("PPT_AUTH"), `require a login, as "user:password" (env PPT_AUTH)`)
	showVersion := fs.Bool("version", false, "print the version and exit")
	_ = fs.Parse(args)

	if *showVersion {
		fmt.Println("pptoolkit", version)
		return nil
	}

	store := style.Store{Dir: *stylesDir}
	if err := store.Seed(style.Defaults()); err != nil {
		return fmt.Errorf("setting up styles folder %s: %w", store.Dir, err)
	}
	cfg := web.Config{Styles: store, Version: version}
	if *auth != "" {
		u, p, ok := strings.Cut(*auth, ":")
		if !ok || u == "" || p == "" {
			return errors.New(`-auth must look like "user:password"`)
		}
		cfg.Username, cfg.Password = u, p
	}
	srv, err := web.New(cfg)
	if err != nil {
		return err
	}

	if cfg.Username == "" {
		log.Printf("No login required - set PPT_AUTH to add one.")
	}
	tlsCfg, err := tlsFromEnv(store.Dir)
	if err != nil {
		return err
	}
	if tlsCfg != nil {
		log.Printf("ProPresenter Toolkit %s (styles: %s)", version, store.Dir)
		return serveTLS(tlsCfg, *addr, srv)
	}
	log.Printf("ProPresenter Toolkit %s listening on %s (styles: %s)", version, *addr, store.Dir)
	hs := &http.Server{Addr: *addr, Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	return hs.ListenAndServe()
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
