package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallCommand(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	if err := run([]string{"install", "-root", root}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "created:") != 2 {
		t.Fatalf("output = %q", out.String())
	}
	for _, name := range []string{".custom-gcl.yml", ".golangci.yml"} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestBudgetCheckAndRaise(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module m\n")
	write("a.go", "package m\n\nvar A = 1\n")
	write(".loc-budgets.json", `{"global": 1, "areas": {}}`)

	var out bytes.Buffer
	if err := run([]string{"budget", "check", "-root", root}, &out); err == nil || !strings.Contains(out.String(), "raise needed: global +1") {
		t.Fatalf("check err=%v out=%q", err, out.String())
	}
	if err := run([]string{"budget", "raise", "-root", root}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run([]string{"budget", "check", "-root", root}, &out); err != nil {
		t.Fatalf("after raise: %v\n%s", err, out.String())
	}
}

func TestBudgetInitSeedsTopLevelDirs(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"go.mod":      "module m\n",
		"a/a.go":      "package a\n\nvar A = 1\n",
		"b/c/b.go":    "package c\n\nvar B = 1\n",
		"vendor/v.go": "package v\n\nvar V = 1\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err := run([]string{"budget", "init", "-root", root}, &out); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, ".loc-budgets.json"))
	for _, want := range []string{`"global": 4`, `"a": 2`, `"b": 2`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("missing %s in %s", want, data)
		}
	}
	if err := run([]string{"budget", "init", "-root", root}, &out); err == nil {
		t.Fatal("second init overwrote the ledger")
	}
}
