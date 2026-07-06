// Package install creates the default lint configuration for a Go project.
package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const Version = "v0.0.2"

type Result struct {
	Path    string
	Created bool
}

var files = []struct {
	name string
	body string
}{
	{".custom-gcl.yml", `version: v2.11.4
name: custom-golangci-lint
destination: .

plugins:
  - module: github.com/antonikliment/go-code-metrics
    import: github.com/antonikliment/go-code-metrics/goclocbudget
    version: v0.0.2
`},
	{".golangci.yml", `version: "2"

linters:
  enable:
    - goclocbudget

  settings:
    custom:
      goclocbudget:
        type: module
        description: Enforces the implementation Go LOC budget using gocloc.
        settings:
          max-go-code-lines: 10000
          include-tests: false
          exclude-generated: true
          exclude-dirs:
            - vendor
            - .git
            - node_modules
            - app/dist
`},
}

// Ensure creates missing lint configuration files without changing existing files.
func Ensure(root string) ([]Result, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("project root is not a directory: %s", root)
	}
	results := make([]Result, 0, len(files))
	for _, file := range files {
		path := filepath.Join(root, file.name)
		created, err := create(path, file.body)
		if err != nil {
			return results, err
		}
		results = append(results, Result{Path: path, Created: created})
	}
	return results, nil
}

func create(path, body string) (bool, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if _, err := file.WriteString(body); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return false, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return false, err
	}
	return true, nil
}
