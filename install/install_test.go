package install

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureCreatesMissingFilesAndPreservesExisting(t *testing.T) {
	root := t.TempDir()
	results, err := Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if !result.Created {
			t.Fatalf("%s was not created", result.Path)
		}
	}
	custom := filepath.Join(root, ".custom-gcl.yml")
	data, err := os.ReadFile(custom)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "version: "+Version) {
		t.Fatalf("custom config does not pin %s:\n%s", Version, data)
	}
	if err := os.WriteFile(custom, []byte("user config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err = Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range results {
		if result.Created {
			t.Fatalf("%s was overwritten", result.Path)
		}
	}
	data, err = os.ReadFile(custom)
	if err != nil || string(data) != "user config\n" {
		t.Fatalf("existing config changed: %q, %v", data, err)
	}
}

func TestEnsureCreatesOnlyMissingFile(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, ".custom-gcl.yml")
	if err := os.WriteFile(custom, []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := Ensure(root)
	if err != nil {
		t.Fatal(err)
	}
	if results[0].Created || !results[1].Created {
		t.Fatalf("results = %+v", results)
	}
}
