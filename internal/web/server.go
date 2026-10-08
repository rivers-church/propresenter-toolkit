// Package web is the browser front end: Convert (PDF -> .pro) and the
// Style Manager (learn a style from an example .pro).
//
// Everything - templates, CSS - is embedded, so the binary is the whole
// app. In-progress conversions live in memory; styles are JSON files on disk.
package web

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rivers-church/propresenter-toolkit/internal/convert"
	"github.com/rivers-church/propresenter-toolkit/internal/pb"
	"github.com/rivers-church/propresenter-toolkit/internal/pro"
	"github.com/rivers-church/propresenter-toolkit/internal/style"
)

//go:embed templates/*.html static/*
var assets embed.FS

const (
	maxUpload  = 64 << 20 // 64 MB
	sessionTTL = 6 * time.Hour
)

// Config configures the server.
type Config struct {
	Styles   style.Store
	Username string // if set (with Password), HTTP basic auth is required
	Password string
	Version  string
}

// Server serves the web UI.
type Server struct {
	cfg   Config
	pages map[string]*template.Template
	mux   *http.ServeMux

	mu       sync.Mutex
	jobs     map[string]*jobSession
	learners map[string]*styleSession
}

type jobSession struct {
	job     *convert.Job
	touched time.Time
}

// styleSession is an in-progress "new style" wizard.
type styleSession struct {
	kind      style.Kind
	pres      *pb.Presentation
	fileName  string
	templates map[string][]byte
	looks     map[string]style.Look
	picked    map[string]int // role -> cue index, for display
	touched   time.Time
}

// New builds a Server.
func New(cfg Config) (*Server, error) {
	s := &Server{
		cfg:      cfg,
		pages:    map[string]*template.Template{},
		mux:      http.NewServeMux(),
		jobs:     map[string]*jobSession{},
		learners: map[string]*styleSession{},
	}
	funcs := template.FuncMap{"join": strings.Join, "inc": func(i int) int { return i + 1 }}
	pages, err := fs.Glob(assets, "templates/*.html")
	if err != nil {
		return nil, err
	}
	for _, p := range pages {
		name := strings.TrimSuffix(strings.TrimPrefix(p, "templates/"), ".html")
		if name == "base" {
			continue
		}
		t, err := template.New("base.html").Funcs(funcs).ParseFS(assets, "templates/base.html", p)
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		s.pages[name] = t
	}
	s.routes()
	go s.janitor()
	return s, nil
}

func (s *Server) routes() {
	static, _ := fs.Sub(assets, "static")
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))

	s.mux.HandleFunc("GET /{$}", s.convertForm)
	s.mux.HandleFunc("POST /convert", s.convertUpload)
	s.mux.HandleFunc("GET /convert/{token}", s.convertReview)
	s.mux.HandleFunc("POST /convert/{token}/remove", s.convertRemove)
	s.mux.HandleFunc("POST /convert/{token}/download", s.convertDownload)

	s.mux.HandleFunc("GET /styles", s.stylesList)
	s.mux.HandleFunc("GET /styles/new", s.styleNewForm)
	s.mux.HandleFunc("POST /styles/new", s.styleNewUpload)
	s.mux.HandleFunc("GET /styles/new/{token}", s.stylePick)
	s.mux.HandleFunc("POST /styles/new/{token}/capture", s.styleCapture)
	s.mux.HandleFunc("POST /styles/new/{token}/save", s.styleSave)
	s.mux.HandleFunc("GET /styles/download", s.styleDownload)
	s.mux.HandleFunc("POST /styles/delete", s.styleDelete)
}

// ServeHTTP implements http.Handler, applying basic auth when configured.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Username != "" || s.cfg.Password != "" {
		u, p, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(u), []byte(s.cfg.Username)) != 1 ||
			subtle.ConstantTimeCompare([]byte(p), []byte(s.cfg.Password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="ProPresenter Toolkit"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	s.mux.ServeHTTP(w, r)
}

// janitor forgets abandoned sessions.
func (s *Server) janitor() {
	for range time.Tick(10 * time.Minute) {
		cutoff := time.Now().Add(-sessionTTL)
		s.mu.Lock()
		for k, v := range s.jobs {
			if v.touched.Before(cutoff) {
				delete(s.jobs, k)
			}
		}
		for k, v := range s.learners {
			if v.touched.Before(cutoff) {
				delete(s.learners, k)
			}
		}
		s.mu.Unlock()
	}
}

// ---------------------------------------------------------------- helpers --

type page struct {
	Title   string
	Nav     string
	Flash   []string
	Version string
	Data    any
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name, title, nav string, data any) {
	t, ok := s.pages[name]
	if !ok {
		http.Error(w, "no such page", http.StatusInternalServerError)
		return
	}
	p := page{Title: title, Nav: nav, Flash: takeFlash(w, r), Version: s.cfg.Version, Data: data}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.Execute(w, p); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

// Flash messages survive one redirect via a short-lived cookie.
func flash(w http.ResponseWriter, msg string) {
	http.SetCookie(w, &http.Cookie{Name: "flash", Value: url.QueryEscape(msg), Path: "/", MaxAge: 60, HttpOnly: true, SameSite: http.SameSiteLaxMode})
}

func takeFlash(w http.ResponseWriter, r *http.Request) []string {
	c, err := r.Cookie("flash")
	if err != nil {
		return nil
	}
	http.SetCookie(w, &http.Cookie{Name: "flash", Path: "/", MaxAge: -1})
	msg, err := url.QueryUnescape(c.Value)
	if err != nil || msg == "" {
		return nil
	}
	return []string{msg}
}

func redirectWith(w http.ResponseWriter, r *http.Request, to, msg string) {
	if msg != "" {
		flash(w, msg)
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func newToken() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func readUpload(w http.ResponseWriter, r *http.Request, field string) ([]byte, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	f, hdr, err := r.FormFile(field)
	if err != nil {
		return nil, "", errors.New("choose a file first")
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, "", err
	}
	return data, hdr.Filename, nil
}

func (s *Server) job(token string) *jobSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	j := s.jobs[token]
	if j != nil {
		j.touched = time.Now()
	}
	return j
}

func (s *Server) learner(token string) *styleSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.learners[token]
	if l != nil {
		l.touched = time.Now()
	}
	return l
}

// ---------------------------------------------------------------- convert --

func (s *Server) convertForm(w http.ResponseWriter, r *http.Request) {
	prompts, _ := s.cfg.Styles.ListKind(style.KindPrompts)
	slides, _ := s.cfg.Styles.ListKind(style.KindSlides)
	s.render(w, r, "convert", "Convert", "convert", map[string]any{
		"PromptsStyles": prompts,
		"SlidesStyles":  slides,
	})
}

func (s *Server) convertUpload(w http.ResponseWriter, r *http.Request) {
	data, _, err := readUpload(w, r, "pdf")
	if err != nil {
		redirectWith(w, r, "/", err.Error())
		return
	}
	mode := style.Kind(r.FormValue("mode"))
	styleName := r.FormValue("style_" + string(mode))
	if styleName == "" {
		redirectWith(w, r, "/", "Choose a style first (or make one in the Style Manager).")
		return
	}
	opt := convert.Options{Colors: convert.ColorMode(r.FormValue("colors"))}
	if n, err := strconv.Atoi(r.FormValue("max_lines")); err == nil && n > 0 {
		opt.MaxLines = n
	}
	if st, err := s.cfg.Styles.Load(styleName); err == nil {
		opt.Style = st
	}
	job, err := convert.Parse(data, mode, opt)
	if err != nil {
		redirectWith(w, r, "/", "Couldn't read that PDF: "+err.Error())
		return
	}
	job.StyleName = styleName
	job.Name = strings.TrimSpace(r.FormValue("name"))
	if job.Name == "" {
		job.Name = "My Message"
	}
	token := newToken()
	s.mu.Lock()
	s.jobs[token] = &jobSession{job: job, touched: time.Now()}
	s.mu.Unlock()
	http.Redirect(w, r, "/convert/"+token, http.StatusSeeOther)
}

func (s *Server) convertReview(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	js := s.job(token)
	if js == nil {
		redirectWith(w, r, "/", "That conversion has expired - start again.")
		return
	}
	s.render(w, r, "review", "Review", "convert", map[string]any{
		"Token":    token,
		"Job":      js.job,
		"Rows":     js.job.Rows(),
		"Warnings": js.job.Warnings(),
	})
}

func (s *Server) convertRemove(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if js := s.job(token); js != nil {
		idx, err := strconv.Atoi(r.FormValue("idx"))
		if err == nil {
			s.mu.Lock()
			js.job.Remove(idx)
			s.mu.Unlock()
		}
	}
	http.Redirect(w, r, "/convert/"+token, http.StatusSeeOther)
}

func (s *Server) convertDownload(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	js := s.job(token)
	if js == nil {
		redirectWith(w, r, "/", "That conversion has expired - start again.")
		return
	}
	st, err := s.cfg.Styles.Load(js.job.StyleName)
	if err != nil {
		redirectWith(w, r, "/convert/"+token, "Couldn't load style: "+err.Error())
		return
	}
	s.mu.Lock()
	js.job.DefaultCopies = r.FormValue("default_copies") != ""
	js.job.DisableCopies = js.job.DefaultCopies && r.FormValue("disable_copies") != ""
	data, _, err := js.job.Build(st)
	s.mu.Unlock()
	if err != nil {
		redirectWith(w, r, "/convert/"+token, "Couldn't build the .pro: "+err.Error())
		return
	}
	name := convert.FileName(js.job.Name)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		strings.ReplaceAll(name, `"`, ""), url.PathEscape(name)))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// ----------------------------------------------------------------- styles --

func (s *Server) stylesList(w http.ResponseWriter, r *http.Request) {
	all, err := s.cfg.Styles.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, r, "styles", "Styles", "styles", map[string]any{"Styles": all, "Dir": s.cfg.Styles.Dir})
}

func (s *Server) styleNewForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "style_new", "New style", "styles", nil)
}

func (s *Server) styleNewUpload(w http.ResponseWriter, r *http.Request) {
	data, fileName, err := readUpload(w, r, "pro")
	if err != nil {
		redirectWith(w, r, "/styles/new", err.Error())
		return
	}
	pres, err := pro.Parse(data)
	if err != nil {
		redirectWith(w, r, "/styles/new", err.Error())
		return
	}
	if len(pres.GetCues()) == 0 {
		redirectWith(w, r, "/styles/new", "That file has no slides in it.")
		return
	}
	kind := style.Kind(r.FormValue("kind"))
	if kind != style.KindPrompts && kind != style.KindSlides {
		kind = style.KindPrompts
	}
	token := newToken()
	s.mu.Lock()
	s.learners[token] = &styleSession{
		kind: kind, pres: pres, fileName: fileName,
		templates: map[string][]byte{}, looks: map[string]style.Look{}, picked: map[string]int{},
		touched: time.Now(),
	}
	s.mu.Unlock()
	http.Redirect(w, r, "/styles/new/"+token, http.StatusSeeOther)
}

type roleButton struct {
	Role     string
	Label    string
	LookOnly bool
}

func (s *Server) stylePick(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	ss := s.learner(token)
	if ss == nil {
		redirectWith(w, r, "/styles/new", "That session has expired - start again.")
		return
	}
	var buttons []roleButton
	if ss.kind == style.KindPrompts {
		buttons = []roleButton{{style.RoleBody, "Body", false}}
	} else {
		buttons = []roleButton{
			{style.RoleTitle, "Title", false},
			{style.RolePoint, "Point", false},
			{style.RoleKeyword, "Keyword", false},
			{style.RoleScripture, "Scripture", false},
			{style.RoleBackTo, "“Back to” look", true},
		}
	}
	type status struct {
		Role, Label string
		Cue         int
		Have        bool
		Look        string
		LookOnly    bool
	}
	s.mu.Lock()
	var st []status
	for _, b := range buttons {
		idx, have := ss.picked[b.Role]
		look := ""
		if l, ok := ss.looks[b.Role]; ok {
			look = l.Name
		}
		st = append(st, status{Role: b.Role, Label: b.Label, Cue: idx + 1, Have: have, Look: look, LookOnly: b.LookOnly})
	}
	s.mu.Unlock()

	settings := style.Settings{}
	s.render(w, r, "style_pick", "New style", "styles", map[string]any{
		"Token":    token,
		"Kind":     string(ss.kind),
		"File":     ss.fileName,
		"Cues":     pro.ListCues(ss.pres),
		"Buttons":  buttons,
		"Status":   st,
		"Regular":  settings.Regular(),
		"Emphasis": settings.Emphasis(),
	})
}

func (s *Server) styleCapture(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	ss := s.learner(token)
	if ss == nil {
		redirectWith(w, r, "/styles/new", "That session has expired - start again.")
		return
	}
	idx, err := strconv.Atoi(r.FormValue("cue"))
	role := r.FormValue("role")
	lookOnly := role == style.RoleBackTo
	if err != nil || idx < 0 || idx >= len(ss.pres.GetCues()) {
		redirectWith(w, r, "/styles/new/"+token, "Pick a slide.")
		return
	}
	cue := ss.pres.GetCues()[idx]
	msg := ""
	s.mu.Lock()
	if !lookOnly {
		if pro.SlideAction(cue) == nil {
			msg = fmt.Sprintf("Slide %d has no slide content to copy.", idx+1)
		} else if data, err := pro.CueBytes(ss.pres, idx); err == nil {
			ss.templates[role] = data
			ss.picked[role] = idx
		}
	} else {
		ss.picked[role] = idx
	}
	if l := pro.AudienceLook(cue); l != nil {
		ss.looks[role] = *l
	} else {
		delete(ss.looks, role)
		if lookOnly {
			msg = fmt.Sprintf("Slide %d has no Audience Look attached.", idx+1)
			delete(ss.picked, role)
		}
	}
	s.mu.Unlock()
	redirectWith(w, r, "/styles/new/"+token, msg)
}

func (s *Server) styleSave(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	ss := s.learner(token)
	if ss == nil {
		redirectWith(w, r, "/styles/new", "That session has expired - start again.")
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		redirectWith(w, r, "/styles/new/"+token, "Give the style a name.")
		return
	}
	s.mu.Lock()
	p := &style.Profile{Name: name, Kind: ss.kind, Templates: map[string][]byte{}}
	for k, v := range ss.templates {
		p.Templates[k] = v
	}
	p.Settings.RegularColor = r.FormValue("regular_color")
	if ss.kind == style.KindPrompts {
		capsOff := r.FormValue("keep_caps") == ""
		p.Settings.EmphasisColor = r.FormValue("emphasis_color")
		p.Settings.ForceCapsOff = &capsOff
		highlight := r.FormValue("highlight_bold") != ""
		p.Settings.HighlightBold = &highlight
	} else {
		p.Settings.AudienceLooks = map[string]style.Look{}
		for k, v := range ss.looks {
			p.Settings.AudienceLooks[k] = v
		}
	}
	s.mu.Unlock()

	if missing := p.Missing(); len(missing) > 0 {
		redirectWith(w, r, "/styles/new/"+token, "Still need to pick: "+strings.Join(missing, ", "))
		return
	}
	if _, err := s.cfg.Styles.Load(name); err == nil && r.FormValue("overwrite") == "" {
		redirectWith(w, r, "/styles/new/"+token, fmt.Sprintf("A style called %q already exists - tick “replace” or choose another name.", name))
		return
	}
	if err := s.cfg.Styles.Save(p); err != nil {
		redirectWith(w, r, "/styles/new/"+token, "Couldn't save: "+err.Error())
		return
	}
	s.mu.Lock()
	delete(s.learners, token)
	s.mu.Unlock()

	msg := fmt.Sprintf("Style %q saved.", name)
	if ss.kind == style.KindSlides {
		var noLook []string
		for _, role := range append(style.KindSlides.RequiredRoles(), style.RoleBackTo) {
			if _, ok := p.Settings.AudienceLooks[role]; !ok {
				noLook = append(noLook, role)
			}
		}
		if len(noLook) > 0 {
			msg += " No Audience Look for: " + strings.Join(noLook, ", ") + " (those slides get no look action)."
		}
	}
	redirectWith(w, r, "/styles", msg)
}

func (s *Server) styleDownload(w http.ResponseWriter, r *http.Request) {
	p, err := s.cfg.Styles.Load(r.FormValue("name"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	data, err := p.JSON()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename*=UTF-8''%s`, url.PathEscape(p.Name+".json")))
	_, _ = w.Write(data)
}

func (s *Server) styleDelete(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if err := s.cfg.Styles.Delete(name); err != nil {
		redirectWith(w, r, "/styles", "Couldn't delete: "+err.Error())
		return
	}
	redirectWith(w, r, "/styles", fmt.Sprintf("Deleted style %q.", name))
}
