// Command pptoolkit turns sermon-notes PDFs into ProPresenter 7 .pro files.
//
//	pptoolkit                       open the app in your browser (local only)
//	pptoolkit serve [flags]         run the web app for other people on the network
//	pptoolkit convert [flags] in.pdf [out.pro]
//	pptoolkit cues file.pro         list the slides in a .pro (to pick templates)
//	pptoolkit version
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/thatguycleeb/propresenter-toolkit/internal/convert"
	"github.com/thatguycleeb/propresenter-toolkit/internal/pro"
	"github.com/thatguycleeb/propresenter-toolkit/internal/style"
	"github.com/thatguycleeb/propresenter-toolkit/internal/web"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	log.SetFlags(0)
	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "":
		err = serve(args, true)
	case "serve":
		err = serve(args, false)
	case "convert":
		err = convertCmd(args)
	case "cues":
		err = cuesCmd(args)
	case "version":
		fmt.Println("pptoolkit", version)
	case "help", "-h", "--help":
		usage()
	default:
		usage()
		err = fmt.Errorf("unknown command %q", cmd)
	}
	if err != nil {
		log.Fatal("error: ", err)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `pptoolkit - sermon-notes PDF to ProPresenter 7 .pro

Usage:
  pptoolkit                         open the app in your browser (this computer only)
  pptoolkit serve [flags]           run the web app for others on the network
  pptoolkit convert [flags] in.pdf [out.pro]
  pptoolkit cues file.pro           list slides in a .pro file
  pptoolkit version

Run "pptoolkit <command> -h" for a command's flags.
`)
}

// defaultStylesDir is where style profiles live unless overridden:
// $PPT_STYLES_DIR, else the per-user config folder
// (e.g. %AppData%\propresenter-toolkit\styles on Windows).
func defaultStylesDir() string {
	if d := os.Getenv("PPT_STYLES_DIR"); d != "" {
		return d
	}
	if d, err := os.UserConfigDir(); err == nil {
		return filepath.Join(d, "propresenter-toolkit", "styles")
	}
	return "styles"
}

func openStore(dir string) (style.Store, error) {
	st := style.Store{Dir: dir}
	if err := st.Seed(style.Defaults()); err != nil {
		return st, fmt.Errorf("setting up styles folder %s: %w", dir, err)
	}
	return st, nil
}

// ------------------------------------------------------------------ serve --

func serve(args []string, desktop bool) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	defAddr := envOr("PPT_ADDR", ":5000")
	if desktop {
		defAddr = "127.0.0.1:5000"
	}
	addr := fs.String("addr", defAddr, "address to listen on (env PPT_ADDR)")
	stylesDir := fs.String("styles", defaultStylesDir(), "folder holding style profiles (env PPT_STYLES_DIR)")
	auth := fs.String("auth", os.Getenv("PPT_AUTH"), `require a login, as "user:password" (env PPT_AUTH)`)
	open := fs.Bool("open", desktop, "open the app in a browser once started")
	_ = fs.Parse(args)

	store, err := openStore(*stylesDir)
	if err != nil {
		return err
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

	ln, err := net.Listen("tcp", *addr)
	if err != nil && desktop {
		// Port taken (maybe the app is already running) - use any free port.
		ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		return err
	}
	url := "http://" + displayAddr(ln.Addr())
	log.Printf("ProPresenter Toolkit %s running at %s", version, url)
	log.Printf("Styles folder: %s", store.Dir)
	if desktop {
		log.Printf("Close this window to stop.")
	}
	if *open {
		go func() {
			time.Sleep(300 * time.Millisecond)
			if err := openBrowser(url); err != nil {
				log.Printf("Open %s in your browser.", url)
			}
		}()
	}
	hs := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	return hs.Serve(ln)
}

func displayAddr(a net.Addr) string {
	host, port, err := net.SplitHostPort(a.String())
	if err != nil {
		return a.String()
	}
	if host == "" || host == "::" || host == "0.0.0.0" {
		host = "localhost"
	}
	return net.JoinHostPort(host, port)
}

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// ---------------------------------------------------------------- convert --

func convertCmd(args []string) error {
	fs := flag.NewFlagSet("convert", flag.ExitOnError)
	mode := fs.String("mode", "prompts", `"prompts" (SLIDE 1: script) or "slides" (labelled outline)`)
	styleName := fs.String("style", "", "style profile to use (see the Style Manager); defaults to the only one of that kind")
	name := fs.String("name", "", "presentation name (default: PDF file name)")
	stylesDir := fs.String("styles", defaultStylesDir(), "folder holding style profiles (env PPT_STYLES_DIR)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: pptoolkit convert [flags] in.pdf [out.pro]")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() < 1 || fs.NArg() > 2 {
		fs.Usage()
		return errors.New("need an input PDF")
	}
	in := fs.Arg(0)
	kind := style.Kind(*mode)
	if kind != style.KindPrompts && kind != style.KindSlides {
		return fmt.Errorf("-mode must be prompts or slides, not %q", *mode)
	}

	store, err := openStore(*stylesDir)
	if err != nil {
		return err
	}
	if *styleName == "" {
		names, err := store.ListKind(kind)
		if err != nil {
			return err
		}
		if len(names) != 1 {
			return fmt.Errorf("choose a style with -style (available %s styles: %s)", kind, strings.Join(names, ", "))
		}
		*styleName = names[0]
	}
	st, err := store.Load(*styleName)
	if err != nil {
		return fmt.Errorf("loading style %q: %w", *styleName, err)
	}

	data, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	job, err := convert.Parse(data, kind)
	if err != nil {
		return err
	}
	job.StyleName = st.Name
	job.Name = *name
	if job.Name == "" {
		job.Name = strings.TrimSuffix(filepath.Base(in), filepath.Ext(in))
	}
	for _, w := range job.Warnings() {
		log.Println("warning:", w)
	}
	out, _, err := job.Build(st)
	if err != nil {
		return err
	}
	outPath := fs.Arg(1)
	if outPath == "" {
		outPath = filepath.Join(filepath.Dir(in), convert.FileName(job.Name))
	}
	if err := os.WriteFile(outPath, out, 0o644); err != nil {
		return err
	}
	log.Printf("Wrote %s (%d slides)", outPath, len(job.Rows()))
	return nil
}

// ------------------------------------------------------------------- cues --

func cuesCmd(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: pptoolkit cues file.pro")
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		return err
	}
	p, err := pro.Parse(data)
	if err != nil {
		return err
	}
	fmt.Printf("%s: %d slides\n", p.GetName(), len(p.GetCues()))
	for _, c := range pro.ListCues(p) {
		look := ""
		if c.Look != nil {
			look = "  [look: " + c.Look.Name + "]"
		}
		fmt.Printf("%4d  %-20s %d box(es)%s\n      %s\n", c.Index+1, c.Name, c.Elements, look, c.Preview)
	}
	return nil
}
