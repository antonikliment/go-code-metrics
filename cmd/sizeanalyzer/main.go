// Command sizeanalyzer reports Go source size and cyclomatic complexity.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/antonikliment/go-code-metrics/analysis"
	"github.com/antonikliment/go-code-metrics/report"
)

func main() {
	var excludeDirs stringList
	root := flag.String("root", ".", "directory to analyze")
	htmlOut := flag.String("html", "", "write an HTML report")
	jsonOut := flag.String("json", "", "write a JSON report")
	top := flag.Int("top", 12, "number of files in top lists")
	includeTests := flag.Bool("include-tests", false, "include _test.go files")
	includeGenerated := flag.Bool("include-generated", false, "include generated Go files")
	hotspots := flag.Int("hotspots", 5, "number of function hotspots per file")
	strict := flag.Bool("strict", false, "fail instead of warning on an unparseable Go file")
	flag.Var(&excludeDirs, "exclude-dir", "exclude a project-relative directory (repeatable)")
	flag.Parse()

	tree, err := analysis.Analyze(analysis.Options{
		Root:             *root,
		IncludeTests:     *includeTests,
		ExcludeGenerated: !*includeGenerated,
		ExcludeDirs:      excludeDirs,
		HotspotLimit:     *hotspots,
		ContinueOnError:  !*strict,
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

type stringList []string

func (values *stringList) String() string {
	return fmt.Sprint([]string(*values))
}

func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func write(path string, data []byte, err error) {
	if err != nil {
		fail("rendering report failed", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fail("writing report failed", err)
	}
}

func fail(message string, err error) {
	fmt.Fprintln(os.Stderr, message+":", err)
	os.Exit(1)
}
