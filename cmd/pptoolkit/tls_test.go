package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedirectToHTTPS(t *testing.T) {
	for _, c := range []struct{ addr, want string }{
		{":443", "https://protoolkit.rivers.church/convert/abc?x=1"},
		{":8443", "https://protoolkit.rivers.church:8443/convert/abc?x=1"},
	} {
		rec := httptest.NewRecorder()
		redirectToHTTPS("protoolkit.rivers.church", c.addr).ServeHTTP(rec, httptest.NewRequest("GET", "http://192.168.16.220/convert/abc?x=1", nil))
		if rec.Code != http.StatusPermanentRedirect || rec.Header().Get("Location") != c.want {
			t.Errorf("%s: got %d %q, want %q", c.addr, rec.Code, rec.Header().Get("Location"), c.want)
		}
	}
}

func TestTLSFromEnv(t *testing.T) {
	t.Setenv("PPT_DOMAIN", "")
	if s, err := tlsFromEnv("/var/lib/x/styles"); s != nil || err != nil {
		t.Errorf("no domain: got %+v, %v; want plain http", s, err)
	}

	t.Setenv("PPT_DOMAIN", "protoolkit.rivers.church")
	t.Setenv("PPT_CLOUDFLARE_API_TOKEN", "")
	if _, err := tlsFromEnv("/var/lib/x/styles"); err == nil {
		t.Error("a domain without a token should be an error")
	}

	t.Setenv("PPT_CLOUDFLARE_API_TOKEN", "test-token")
	t.Setenv("PPT_CERT_DIR", "")
	s, err := tlsFromEnv("/var/lib/x/styles")
	if err != nil || s == nil {
		t.Fatalf("got %+v, %v", s, err)
	}
	if s.HTTPSAddr != ":443" || s.CertDir == "" {
		t.Errorf("defaults wrong: %+v", s)
	}
}
