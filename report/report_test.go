package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/antonikliment/goclocbudget/analysis"
)

func TestRenderers(t *testing.T) {
	root := &analysis.Node{Name: "demo", Path: ".", Code: 10, Complexity: 3, Functions: 1, Children: []*analysis.Node{
		{Name: "main.go", Path: "main.go", IsFile: true, Code: 10, Complexity: 3, MaxComplexity: 3, Functions: 1},
	}}
	terminal := Terminal(root, 1)
	if !strings.Contains(terminal, "10 code") || !strings.Contains(terminal, "main.go") {
		t.Fatalf("terminal report missing metrics:\n%s", terminal)
	}
	data, err := JSON(root)
	if err != nil || !json.Valid(data) {
		t.Fatalf("JSON: %v", err)
	}
	html, err := HTML(root)
	if err != nil || strings.Contains(string(html), dataPlaceholder) || !strings.Contains(string(html), `"name":"demo"`) {
		t.Fatalf("HTML: %v", err)
	}
}
