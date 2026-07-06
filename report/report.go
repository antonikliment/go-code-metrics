// Package report renders analysis results without coupling presentation to metrics.
package report

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/antonikliment/go-code-metrics/analysis"
)

//go:embed report.html
var htmlTemplate string

const dataPlaceholder = "/*__DATA__*/null"

func JSON(root *analysis.Node) ([]byte, error) {
	return json.MarshalIndent(root, "", "  ")
}

func HTML(root *analysis.Node) ([]byte, error) {
	data, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	return []byte(strings.Replace(htmlTemplate, dataPlaceholder, string(data), 1)), nil
}

func Terminal(root *analysis.Node, top int) string {
	files := analysis.Files(root)
	var out strings.Builder
	fmt.Fprintf(&out, "Project size analysis — %s\n", root.Name)
	fmt.Fprintf(&out, "%d files · %d code · %d comment · %d blank · complexity %d (avg %.1f over %d funcs)\n\n",
		len(files), root.Code, root.Comment, root.Blank, root.Complexity, average(root.Complexity, root.Functions), root.Functions)
	for _, warning := range root.Warnings {
		fmt.Fprintf(&out, "warning: %s\n", warning)
	}
	if len(root.Warnings) > 0 {
		out.WriteByte('\n')
	}
	writeTree(&out, root, root.Code, "")
	fmt.Fprintf(&out, "\nTop %d files by code:\n", top)
	writeFiles(&out, topBy(files, top, func(node *analysis.Node) int { return node.Code }))
	fmt.Fprintf(&out, "\nTop %d files by complexity:\n", top)
	writeFiles(&out, topBy(files, top, func(node *analysis.Node) int { return node.Complexity }))
	return out.String()
}

func writeTree(out *strings.Builder, node *analysis.Node, max int, indent string) {
	fmt.Fprintf(out, "%-30s %s %6d  c%d\n", truncate(indent+node.Name+"/", 30), bar(node.Code, max), node.Code, node.Complexity)
	for _, child := range node.Children {
		if !child.IsFile {
			writeTree(out, child, max, indent+"  ")
		}
	}
}

func writeFiles(out *strings.Builder, files []*analysis.Node) {
	for _, file := range files {
		fmt.Fprintf(out, "  %6d loc  c%-4d  %s\n", file.Code, file.Complexity, file.Path)
	}
}

func topBy(files []*analysis.Node, limit int, value func(*analysis.Node) int) []*analysis.Node {
	out := append([]*analysis.Node(nil), files...)
	sort.SliceStable(out, func(i, j int) bool { return value(out[i]) > value(out[j]) })
	if limit >= 0 && limit < len(out) {
		out = out[:limit]
	}
	return out
}

func bar(value, max int) string {
	const width = 20
	filled := 0
	if max > 0 {
		filled = min(value*width/max, width)
	}
	return strings.Repeat("█", filled) + strings.Repeat("·", width-filled)
}

func truncate(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	return string(runes[:width-1]) + "…"
}

func average(sum, count int) float64 {
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count)
}
