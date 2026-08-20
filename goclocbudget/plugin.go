package goclocbudget

import (
	"fmt"
	"os"
	"path"
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
	MaxGoCodeLines   int               `json:"max-go-code-lines"`
	PathBudgets      map[string]budget `json:"path-budgets"`
	IncludeTests     bool              `json:"include-tests"`
	ExcludeGenerated *bool             `json:"exclude-generated"`
	ExcludeDirs      []string          `json:"exclude-dirs"`
}

type budget struct {
	MaxGoCodeLines int `json:"max-go-code-lines"`
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
	pathBudgets := make(map[string]budget, len(cfg.PathBudgets))
	for configuredPath, budget := range cfg.PathBudgets {
		cleanPath, err := cleanBudgetPath(configuredPath)
		if err != nil {
			return nil, err
		}
		if budget.MaxGoCodeLines <= 0 {
			return nil, fmt.Errorf("path-budgets.%s.max-go-code-lines must be positive", configuredPath)
		}
		if _, exists := pathBudgets[cleanPath]; exists {
			return nil, fmt.Errorf("duplicate path budget after cleaning: %s", cleanPath)
		}
		pathBudgets[cleanPath] = budget
	}
	cfg.PathBudgets = pathBudgets
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
		root, err := repositoryRoot(".")
		if err != nil {
			p.runErr = err
			return
		}
		result, err := p.count(root)
		if err != nil {
			p.runErr = err
			return
		}
		paths := make([]string, 0, len(p.settings.PathBudgets))
		for budgetPath := range p.settings.PathBudgets {
			paths = append(paths, budgetPath)
		}
		sort.Strings(paths)
		for _, budgetPath := range paths {
			limit := p.settings.PathBudgets[budgetPath].MaxGoCodeLines
			if result.paths[budgetPath] > limit {
				pass.Reportf(pass.Files[0].Package, "%s: %d / %d LOC", budgetPath, result.paths[budgetPath], limit)
			}
		}
		if result.code > p.settings.MaxGoCodeLines {
			pass.Reportf(pass.Files[0].Package, "repository: %d / %d LOC. Largest files: %s",
				result.code, p.settings.MaxGoCodeLines, strings.Join(result.largest, ", "))
		}
	})
	return nil, p.runErr
}

type countResult struct {
	code    int
	largest []string
	paths   map[string]int
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
	pathCounts := make(map[string]int, len(p.settings.PathBudgets))
	for _, file := range files {
		for budgetPath := range p.settings.PathBudgets {
			if file.Path == budgetPath || strings.HasPrefix(file.Path, budgetPath+"/") {
				pathCounts[budgetPath] += file.Code
			}
		}
	}
	return countResult{code: tree.Code, largest: largestFileSummary(files, 5), paths: pathCounts}, nil
}

func repositoryRoot(start string) (string, error) {
	root, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	var module string
	for {
		if exists, err := pathExists(filepath.Join(root, "go.work")); err != nil {
			return "", err
		} else if exists {
			return root, nil
		}
		if module == "" {
			if exists, err := pathExists(filepath.Join(root, "go.mod")); err != nil {
				return "", err
			} else if exists {
				module = root
			}
		}
		if exists, err := gitRootExists(root); err != nil {
			return "", err
		} else if exists {
			return root, nil
		}
		parent := filepath.Dir(root)
		if parent == root {
			if module != "" {
				return module, nil
			}
			return "", fmt.Errorf("go.mod, go.work, or .git not found from %s", start)
		}
		root = parent
	}
}

func gitRootExists(root string) (bool, error) {
	info, err := os.Stat(filepath.Join(root, ".git"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return true, nil
	}
	return pathExists(filepath.Join(root, ".git", "HEAD"))
}

func pathExists(name string) (bool, error) {
	_, err := os.Stat(name)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

func cleanBudgetPath(configuredPath string) (string, error) {
	cleanPath := path.Clean(strings.TrimSpace(strings.ReplaceAll(configuredPath, `\`, "/")))
	if cleanPath == "." || path.IsAbs(cleanPath) || strings.HasPrefix(cleanPath, "../") {
		return "", fmt.Errorf("path budget must be a relative path below the repository root: %q", configuredPath)
	}
	return strings.TrimPrefix(cleanPath, "./"), nil
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
