package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/antonikliment/go-code-metrics/analysis"
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

func TestTerminalNegativeTopMeansAll(t *testing.T) {
	root := &analysis.Node{Name: "demo", Path: ".", Code: 1, Children: []*analysis.Node{
		{Name: "main.go", Path: "main.go", IsFile: true, Code: 1},
	}}
	if output := Terminal(root, -1); !strings.Contains(output, "main.go") {
		t.Fatalf("negative top omitted files:\n%s", output)
	}
}

func TestHTMLEscapesScriptClosingTags(t *testing.T) {
	root := &analysis.Node{Name: "</script><script>alert(1)</script>", Path: "."}
	html, err := HTML(root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(html), root.Name) || !strings.Contains(string(html), `\u003c/script\u003e`) {
		t.Fatalf("unsafe embedded JSON: %s", html)
	}
}

func TestTruncatePreservesRunes(t *testing.T) {
	if got := truncate("ab界cd", 4); got != "ab界…" {
		t.Fatalf("truncate = %q", got)
	}
}
