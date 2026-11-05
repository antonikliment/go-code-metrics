package budget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, body := range files {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func twoLines(name string) string { return "package p\n\nvar " + name + " = 1\n" }

func TestMeasureAssignsLongestPrefix(t *testing.T) {
	root := writeTree(t, map[string]string{
		"main.go":          twoLines("M"),
		"cmd/a.go":         twoLines("A"),
		"cmd/ui/b.go":      twoLines("B"),
		"cmd/ui/b_test.go": twoLines("T"),
	})
	l := Ledger{Global: 10, Areas: map[string]int{"cmd": 5, "cmd/ui": 5}}
	u, err := Measure(root, l)
	if err != nil {
		t.Fatal(err)
	}
	if u.Global != 6 || u.Areas["cmd"] != 2 || u.Areas["cmd/ui"] != 2 || u.Areas[Root] != 2 {
		t.Fatalf("usage = %+v", u)
	}
}

func TestCheckAndRaise(t *testing.T) {
	l := Ledger{Global: 5, Areas: map[string]int{"a": 3, "b": 3}}
	u := Usage{Global: 6, Areas: map[string]int{"a": 4, "b": 1}}

	over := Check(u, l)
	if len(over) != 2 || over[0].Area != "global" || over[1].Area != "a" || over[1].Raise() != 1 {
		t.Fatalf("overruns = %+v", over)
	}

	raised, changed := Raise(l, u)
	if !changed || raised.Global != 6 || raised.Areas["a"] != 4 || raised.Areas["b"] != 3 {
		t.Fatalf("raised = %+v changed=%v", raised, changed)
	}
	if l.Areas["a"] != 3 {
		t.Fatal("Raise mutated its input")
	}
	if _, changed := Raise(raised, u); changed {
		t.Fatal("second raise reported a change")
	}
}

func TestLedgerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger.json")
	want := Ledger{Global: 9, Areas: map[string]int{"x": 4}, ExcludeDirs: []string{"vendor"}}
	if err := want.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Global != 9 || got.Areas["x"] != 4 || got.ExcludeDirs[0] != "vendor" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestRenderMarkdownDelta(t *testing.T) {
	var b strings.Builder
	l := Ledger{Global: 10, Areas: map[string]int{"a": 5}}
	err := RenderMarkdown(&b, l, Usage{Global: 7, Areas: map[string]int{"a": 4}},
		&Usage{Global: 6, Areas: map[string]int{"a": 2}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"| `global` | 7 | 10 | 3 | +1 |", "| `a` | 4 | 5 | 1 | +2 |"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("missing %q in:\n%s", want, b.String())
		}
	}
}
