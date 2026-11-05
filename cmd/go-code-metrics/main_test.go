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
