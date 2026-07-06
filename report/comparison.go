package report

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/antonikliment/go-code-metrics/comparison"
)

//go:embed comparison.html
var comparisonTemplate string

func ComparisonJSON(result *comparison.Result) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

func ComparisonHTML(result *comparison.Result) ([]byte, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return []byte(strings.Replace(comparisonTemplate, dataPlaceholder, string(data), 1)), nil
}

func ComparisonTerminal(result *comparison.Result, top int) string {
	var out strings.Builder
	fmt.Fprintf(&out, "PR code analysis — base %s (%s)\n", result.Base, shortRevision(result.MergeBase))
	fmt.Fprintf(&out, "%d changed Go files · lines +%d -%d · code %s · complexity +%d -%d (net %s)\n\n",
		len(result.Files), result.LinesAdded, result.LinesDeleted, signed(result.CodeAfter-result.CodeBefore),
		result.ComplexityAdded, result.ComplexityRemoved, signed(result.ComplexityAfter-result.ComplexityBefore))
	for _, warning := range result.Warnings {
		fmt.Fprintf(&out, "warning: %s\n", warning)
	}
	if len(result.Warnings) > 0 {
		out.WriteByte('\n')
	}
	files := result.Files
	if top >= 0 && top < len(files) {
		files = files[:top]
	}
	for _, file := range files {
		fmt.Fprintf(&out, "%-9s +%-3d -%-3d  c +%-3d -%-3d  %s\n",
			file.Status, file.LinesAdded, file.LinesDeleted, file.ComplexityAdded, file.ComplexityRemoved, file.Path)
		for _, function := range file.Functions {
			fmt.Fprintf(&out, "  %-8s c%d → c%d (%s) %s\n", function.Status, function.Before, function.After, signed(function.Delta), function.Name)
		}
	}
	return out.String()
}

func signed(value int) string {
	if value >= 0 {
		return fmt.Sprintf("+%d", value)
	}
	return fmt.Sprint(value)
}

func shortRevision(revision string) string {
	if len(revision) > 12 {
		return revision[:12]
	}
	return revision
}
