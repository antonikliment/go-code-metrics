package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/antonikliment/go-code-metrics/comparison"
)

func TestComparisonRenderers(t *testing.T) {
	result := &comparison.Result{Base: "main", MergeBase: "1234567890abcdef", LinesAdded: 3, ComplexityAdded: 2, Files: []comparison.FileChange{
		{Path: "</script>.go", Status: "modified", LinesAdded: 3, ComplexityAdded: 2},
	}}
	terminal := ComparisonTerminal(result, 10)
	if !strings.Contains(terminal, "complexity +2") || !strings.Contains(terminal, result.Files[0].Path) {
		t.Fatalf("terminal report missing metrics:\n%s", terminal)
	}
	data, err := ComparisonJSON(result)
	if err != nil || !json.Valid(data) {
		t.Fatalf("JSON: %v", err)
	}
	html, err := ComparisonHTML(result)
	if err != nil || strings.Contains(string(html), result.Files[0].Path) || !strings.Contains(string(html), `\u003c/script\u003e.go`) {
		t.Fatalf("unsafe HTML: %v\n%s", err, html)
	}
}
