package goclocbudget

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	metrics "github.com/antonikliment/go-code-metrics/analysis"
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

const pluginName = "goclocbudget"

func init() {
	register.Plugin(pluginName, New)
}

type settings struct {
	MaxGoCodeLines   int      `json:"max-go-code-lines"`
	IncludeTests     bool     `json:"include-tests"`
	ExcludeGenerated bool     `json:"exclude-generated"`
	ExcludeDirs      []string `json:"exclude-dirs"`
}

type plugin struct {
	settings settings
}

func New(raw any) (register.LinterPlugin, error) {
	cfg, err := register.DecodeSettings[settings](raw)
	if err != nil {
		return nil, err
	}
	if cfg.MaxGoCodeLines <= 0 {
		return nil, fmt.Errorf("max-go-code-lines must be positive")
	}
	if len(cfg.ExcludeDirs) == 0 {
		cfg.ExcludeDirs = []string{"vendor", ".git", "node_modules", "app/dist"}
	}
	return plugin{settings: cfg}, nil
}

func (p plugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

func (p plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: pluginName,
		Doc:  "checks repository-wide implementation Go code line budget",
		Run:  p.run,
	}}, nil
}

func (p plugin) run(pass *analysis.Pass) (any, error) {
	if !isRootPackage(pass) {
		return nil, nil
	}
	result, err := p.count(".")
	if err != nil {
		return nil, err
	}
	if result.code <= p.settings.MaxGoCodeLines {
		return nil, nil
	}
	pass.Reportf(pass.Files[0].Package, "implementation Go LOC budget exceeded: %d > %d. Largest files: %s",
		result.code, p.settings.MaxGoCodeLines, strings.Join(result.largest, ", "))
	return nil, nil
}

type countResult struct {
	code    int
	largest []string
}

func (p plugin) count(root string) (countResult, error) {
	tree, err := metrics.Analyze(metrics.Options{
		Root:             root,
		IncludeTests:     p.settings.IncludeTests,
		ExcludeGenerated: p.settings.ExcludeGenerated,
		ExcludeDirs:      p.settings.ExcludeDirs,
	})
	if err != nil {
		return countResult{}, err
	}
	files := analysisFiles(tree)
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Code != files[j].Code {
			return files[i].Code > files[j].Code
		}
		return files[i].Path < files[j].Path
	})
	return countResult{code: tree.Code, largest: largestFileSummary(files, 5)}, nil
}

func isRootPackage(pass *analysis.Pass) bool {
	for _, file := range pass.Files {
		if filepath.Base(pass.Fset.File(file.Package).Name()) == "main.go" {
			return true
		}
	}
	return false
}

func analysisFiles(node *metrics.Node) []*metrics.Node {
	if node.IsFile {
		return []*metrics.Node{node}
	}
	var files []*metrics.Node
	for _, child := range node.Children {
		files = append(files, analysisFiles(child)...)
	}
	return files
}

func largestFileSummary(files []*metrics.Node, limit int) []string {
	if len(files) < limit {
		limit = len(files)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, fmt.Sprintf("%s=%d", files[i].Path, files[i].Code))
	}
	return out
}
