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
