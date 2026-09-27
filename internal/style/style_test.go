package style

import (
	"bytes"
	"encoding/json"
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
	if err != nil || len(names) != 1 {
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
	// Seeding again must not overwrite user styles.
	if err := s.Seed(map[string][]byte{"x": []byte(`{"name":"X","kind":"prompts"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load("X"); err == nil {
		t.Error("Seed overwrote a non-empty store")
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
