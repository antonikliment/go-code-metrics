package goclocbudget

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

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
	ExcludeGenerated *bool    `json:"exclude-generated"`
	ExcludeDirs      []string `json:"exclude-dirs"`
}

type plugin struct {
	settings settings
	once     sync.Once
	runErr   error
}

func New(raw any) (register.LinterPlugin, error) {
	cfg, err := register.DecodeSettings[settings](raw)
	if err != nil {
		return nil, err
	}
	if cfg.MaxGoCodeLines <= 0 {
		return nil, fmt.Errorf("max-go-code-lines must be positive")
	}
	return &plugin{settings: cfg}, nil
}

func (p *plugin) GetLoadMode() string {
	return register.LoadModeSyntax
}

func (p *plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{{
		Name: pluginName,
		Doc:  "checks repository-wide implementation Go code line budget",
		Run:  p.run,
	}}, nil
}

func (p *plugin) run(pass *analysis.Pass) (any, error) {
	p.once.Do(func() {
		root, err := moduleRoot(".")
		if err != nil {
			p.runErr = err
			return
		}
		result, err := p.count(root)
		if err != nil {
			p.runErr = err
			return
		}
		if result.code > p.settings.MaxGoCodeLines {
			pass.Reportf(pass.Files[0].Package, "implementation Go LOC budget exceeded: %d > %d. Largest files: %s",
				result.code, p.settings.MaxGoCodeLines, strings.Join(result.largest, ", "))
		}
	})
	return nil, p.runErr
}

type countResult struct {
	code    int
	largest []string
}

func (p *plugin) count(root string) (countResult, error) {
	tree, err := metrics.Analyze(metrics.Options{
		Root:             root,
		IncludeTests:     p.settings.IncludeTests,
		ExcludeGenerated: p.settings.ExcludeGenerated == nil || *p.settings.ExcludeGenerated,
		ExcludeDirs:      p.settings.ExcludeDirs,
	})
	if err != nil {
		return countResult{}, err
	}
	files := metrics.Files(tree)
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Code != files[j].Code {
			return files[i].Code > files[j].Code
		}
		return files[i].Path < files[j].Path
	})
	return countResult{code: tree.Code, largest: largestFileSummary(files, 5)}, nil
}

func moduleRoot(start string) (string, error) {
	root, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(root)
		if parent == root {
			return "", fmt.Errorf("go.mod not found from %s", start)
		}
		root = parent
	}
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
