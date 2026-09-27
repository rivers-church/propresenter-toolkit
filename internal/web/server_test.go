package web

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thatguycleeb/propresenter-toolkit/internal/pb"
	"github.com/thatguycleeb/propresenter-toolkit/internal/pro"
	"github.com/thatguycleeb/propresenter-toolkit/internal/style"
	"github.com/thatguycleeb/propresenter-toolkit/internal/testpdf"
)

func newTestServer(t *testing.T, cfg Config) (*httptest.Server, *http.Client) {
	t.Helper()
	if cfg.Styles.Dir == "" {
		cfg.Styles = style.Store{Dir: t.TempDir()}
	}
	srv, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	return ts, &http.Client{Jar: jar}
}

func upload(t *testing.T, c *http.Client, u string, fields map[string]string, fileField, fileName string, file []byte) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile(fileField, fileName)
	_, _ = fw.Write(file)
	_ = mw.Close()
	resp, err := c.Post(u, mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func body(t *testing.T, r *http.Response) string {
	t.Helper()
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return string(b)
}

// examplePro builds a .pro whose single cue is the default prompts style's
// body template - standing in for a real show file.
func examplePro(t *testing.T) []byte {
	t.Helper()
	var tmpl []byte
	for _, data := range style.Defaults() {
		p, _ := style.Parse(data)
		if p.Kind == style.KindPrompts {
			tmpl = p.Templates[style.RoleBody]
		}
	}
	cue := &pb.Cue{}
	if err := proto.Unmarshal(tmpl, cue); err != nil {
		t.Fatal(err)
	}
	data, err := proto.Marshal(&pb.Presentation{Name: "Example", Cues: []*pb.Cue{cue}})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestLearnStyleThenConvert(t *testing.T) {
	ts, c := newTestServer(t, Config{})

	// Style Manager: upload an example, pick the body slide, save.
	resp := upload(t, c, ts.URL+"/styles/new", map[string]string{"kind": "prompts"}, "pro", "Example.pro", examplePro(t))
	if !strings.Contains(resp.Request.URL.Path, "/styles/new/") {
		t.Fatalf("expected redirect to picker, got %s: %s", resp.Request.URL, body(t, resp))
	}
	pick := resp.Request.URL.String()
	body(t, resp)
	resp, _ = c.PostForm(pick+"/capture", url.Values{"cue": {"0"}, "role": {"body"}})
	if b := body(t, resp); !strings.Contains(b, "slide 1") {
		t.Fatalf("capture didn't register: %s", b)
	}
	resp, _ = c.PostForm(pick+"/save", url.Values{"name": {"Test Style"}, "regular_color": {"#FFFFFF"}, "emphasis_color": {"#00FF00"}})
	if b := body(t, resp); !strings.Contains(b, `Style &#34;Test Style&#34; saved.`) {
		t.Fatalf("save failed: %s", b)
	}

	// Convert: upload a PDF, review, download.
	pdf := testpdf.Build([]testpdf.Line{
		{testpdf.Regular("SLIDE 1:")},
		{testpdf.Regular("We are "), testpdf.Bold("SAVED")},
		{testpdf.Regular("SLIDE 2:")},
		{testpdf.Regular("By grace")},
	})
	resp = upload(t, c, ts.URL+"/convert", map[string]string{"mode": "prompts", "style_prompts": "Test Style", "name": "Sunday"}, "pdf", "notes.pdf", pdf)
	review := body(t, resp)
	if !strings.Contains(review, "2 slides found") || !strings.Contains(review, "We are SAVED") {
		t.Fatalf("review page wrong: %s", review)
	}
	token := strings.TrimPrefix(resp.Request.URL.Path, "/convert/")

	resp, _ = c.PostForm(ts.URL+"/convert/"+token+"/remove", url.Values{"idx": {"1"}})
	if b := body(t, resp); !strings.Contains(b, "1 slides found") {
		t.Fatalf("remove didn't work: %s", b)
	}

	resp, _ = c.PostForm(ts.URL+"/convert/"+token+"/download", nil)
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, `Sunday.pro`) {
		t.Errorf("Content-Disposition = %q", cd)
	}
	p, err := pro.Parse([]byte(body(t, resp)))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.GetCues()) != 1 || p.GetName() != "Sunday" {
		t.Fatalf("got %d cues named %q", len(p.GetCues()), p.GetName())
	}
	rtfData := string(pro.SlideAction(p.GetCues()[0]).GetSlide().GetPresentation().GetBaseSlide().GetElements()[0].GetElement().GetText().GetRtfData())
	if !strings.Contains(rtfData, `\red0\green255\blue0`) {
		t.Errorf("emphasis color not applied: %s", rtfData)
	}
}

func TestPagesRender(t *testing.T) {
	ts, c := newTestServer(t, Config{})
	for _, path := range []string{"/", "/styles", "/styles/new", "/static/style.css"} {
		resp, err := c.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != 200 {
			t.Errorf("%s: status %d", path, resp.StatusCode)
		}
		body(t, resp)
	}
}

func TestBadUploads(t *testing.T) {
	ts, c := newTestServer(t, Config{})
	resp := upload(t, c, ts.URL+"/convert", map[string]string{"mode": "prompts", "style_prompts": "x"}, "pdf", "x.pdf", []byte("not a pdf"))
	if b := body(t, resp); !strings.Contains(b, "Couldn&#39;t read that PDF") {
		t.Errorf("bad PDF not reported: %s", b)
	}
	resp = upload(t, c, ts.URL+"/styles/new", map[string]string{"kind": "prompts"}, "pro", "x.pro", []byte{0xff, 0xff, 0xff})
	if b := body(t, resp); !strings.Contains(b, "not a ProPresenter 7 .pro file") {
		t.Errorf("bad .pro not reported: %s", b)
	}
	resp, _ = c.Get(ts.URL + "/convert/doesnotexist")
	if b := body(t, resp); !strings.Contains(b, "expired") {
		t.Errorf("missing session not reported: %s", b)
	}
}

func TestBasicAuth(t *testing.T) {
	ts, c := newTestServer(t, Config{Username: "team", Password: "secret"})
	resp, _ := c.Get(ts.URL + "/")
	body(t, resp)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no credentials: status %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("GET", ts.URL+"/", nil)
	req.SetBasicAuth("team", "secret")
	resp, _ = c.Do(req)
	body(t, resp)
	if resp.StatusCode != 200 {
		t.Errorf("with credentials: status %d", resp.StatusCode)
	}
}
