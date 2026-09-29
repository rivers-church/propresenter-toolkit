package style

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// The bundled defaults were written by the original Python toolkit; they
// must load, and re-saving must not lose anything.
func TestDefaultsRoundTrip(t *testing.T) {
	defaults := Defaults()
	if len(defaults) == 0 {
		t.Fatal("no bundled default styles")
	}
	for file, data := range defaults {
		p, err := Parse(data)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if missing := p.Missing(); len(missing) > 0 {
			t.Errorf("%s: missing roles %v", file, missing)
		}
		for _, role := range p.Kind.RequiredRoles() {
			if _, err := p.TemplateCue(role); err != nil {
				t.Errorf("%s: %v", file, err)
			}
		}
		out, err := p.JSON()
		if err != nil {
			t.Fatal(err)
		}
		var a, b any
		_ = json.Unmarshal(data, &a)
		_ = json.Unmarshal(out, &b)
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b)
		if !bytes.Equal(ja, jb) {
			t.Errorf("%s: re-saved JSON differs from original", file)
		}
	}
}

func TestStore(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.Seed(Defaults()); err != nil {
		t.Fatal(err)
	}
	names, err := s.ListKind(KindPrompts)
	want := []string{"Message (Prompts)", "Message (Prompts, all yellow)", "Message (Prompts, plain)"}
	if err != nil || strings.Join(names, "|") != strings.Join(want, "|") {
		t.Fatalf("prompts styles = %v, %v", names, err)
	}
	p, err := s.Load(names[0])
	if err != nil {
		t.Fatal(err)
	}
	p.Name = "Copy"
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("Copy"); err != nil {
		t.Fatal(err)
	}
}

func TestSeedOnlyOffersEachDefaultOnce(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	a := []byte(`{"name":"A","kind":"prompts"}`)
	b := []byte(`{"name":"B","kind":"prompts"}`)
	if err := s.Seed(map[string][]byte{"a": a}); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("A"); err != nil {
		t.Fatal(err)
	}
	// A later version adds B; the deleted A must not come back.
	if err := s.Seed(map[string][]byte{"a": a, "b": b}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("A"); err == nil {
		t.Error("deleted default A was restored")
	}
	if _, err := s.Load("B"); err != nil {
		t.Errorf("new default B not added: %v", err)
	}
}

func TestSeedNeverOverwrites(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	mine := &Profile{Name: "A", Kind: KindSlides}
	if err := s.Save(mine); err != nil {
		t.Fatal(err)
	}
	if err := s.Seed(map[string][]byte{"a": []byte(`{"name":"A","kind":"prompts"}`)}); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Load("A"); p == nil || p.Kind != KindSlides {
		t.Errorf("user's style A was overwritten: %+v", p)
	}
}

// Installs from before the offered-list existed already got the original
// examples; a deleted original must not reappear, but new ones should.
func TestSeedLegacyStore(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	if err := s.Save(&Profile{Name: "My own", Kind: KindPrompts}); err != nil {
		t.Fatal(err)
	}
	if err := s.Seed(Defaults()); err != nil {
		t.Fatal(err)
	}
	for _, name := range legacyDefaults {
		if _, err := s.Load(name); err == nil {
			t.Errorf("legacy default %q was re-added", name)
		}
	}
	if _, err := s.Load("Message (Prompts, plain)"); err != nil {
		t.Errorf("new default not added to legacy store: %v", err)
	}
}

func TestStoreRejectsBadNames(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	for _, name := range []string{"", "../evil", `a\b`, "..", "x:y"} {
		if err := s.Save(&Profile{Name: name, Kind: KindPrompts}); err == nil {
			t.Errorf("Save(%q) should fail", name)
		}
	}
}

func TestSettingsDefaults(t *testing.T) {
	var s Settings
	if s.Regular() != "#FFFFFF" || s.Emphasis() != "#FFFF00" || !s.CapsOff() {
		t.Errorf("defaults wrong: %+v", s)
	}
	off := false
	s.ForceCapsOff = &off
	if s.CapsOff() {
		t.Error("explicit false ignored")
	}
}
