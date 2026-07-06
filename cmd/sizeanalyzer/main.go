// Command sizeanalyzer reports Go source size and cyclomatic complexity.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/antonikliment/goclocbudget/analysis"
	"github.com/antonikliment/goclocbudget/report"
)

func main() {
	root := flag.String("root", ".", "directory to analyze")
	htmlOut := flag.String("html", "", "write an HTML report")
	jsonOut := flag.String("json", "", "write a JSON report")
	top := flag.Int("top", 12, "number of files in top lists")
	includeTests := flag.Bool("include-tests", false, "include _test.go files")
	includeGenerated := flag.Bool("include-generated", false, "include generated Go files")
	flag.Parse()

	tree, err := analysis.Analyze(analysis.Options{
		Root:             *root,
		IncludeTests:     *includeTests,
		ExcludeGenerated: !*includeGenerated,
	})
	if err != nil {
		fail("analysis failed", err)
	}
	fmt.Print(report.Terminal(tree, *top))
	if *jsonOut != "" {
		data, err := report.JSON(tree)
		write(*jsonOut, data, err)
	}
	if *htmlOut != "" {
		data, err := report.HTML(tree)
		write(*htmlOut, data, err)
	}
}

func write(path string, data []byte, err error) {
	if err != nil {
		fail("rendering report failed", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		fail("writing report failed", err)
	}
}

func fail(message string, err error) {
	fmt.Fprintln(os.Stderr, message+":", err)
	os.Exit(1)
}
