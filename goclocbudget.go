package goclocbudget

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/golangci/plugin-module-register/register"
	"github.com/hhatto/gocloc"
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
	opts := gocloc.NewClocOptions()
	opts.IncludeLangs["Go"] = struct{}{}
	opts.Fullpath = true
	opts.ReNotMatchDir = excludeDirRegexp(p.settings.ExcludeDirs)
	if !p.settings.IncludeTests {
		opts.ReNotMatch = regexp.MustCompile(`_test\.go$`)
	}
	processor := gocloc.NewProcessor(gocloc.NewDefinedLanguages(), opts)
	result, err := processor.Analyze([]string{root})
	if err != nil {
		return countResult{}, err
	}
	files := make(gocloc.ClocFiles, 0, len(result.Files))
	for path, file := range result.Files {
		if p.settings.ExcludeGenerated && isGenerated(path) {
			continue
		}
		files = append(files, *file)
	}
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Code != files[j].Code {
			return files[i].Code > files[j].Code
		}
		return files[i].Name < files[j].Name
	})
	code := 0
	for _, file := range files {
		code += int(file.Code)
	}
	return countResult{code: code, largest: largestFileSummary(files, 5)}, nil
}

func isRootPackage(pass *analysis.Pass) bool {
	for _, file := range pass.Files {
		if filepath.Base(pass.Fset.File(file.Package).Name()) == "main.go" {
			return true
		}
	}
	return false
}

func excludeDirRegexp(dirs []string) *regexp.Regexp {
	parts := make([]string, 0, len(dirs))
	for _, dir := range dirs {
		dir = strings.Trim(strings.TrimSpace(filepath.ToSlash(dir)), "/")
		if dir != "" {
			parts = append(parts, regexp.QuoteMeta(dir))
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return regexp.MustCompile(`(^|/)` + `(` + strings.Join(parts, "|") + `)(/|$)`)
}

func isGenerated(path string) bool {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return false
	}
	for _, line := range strings.SplitN(string(data), "\n", 12) {
		if strings.HasPrefix(line, "// Code generated ") && strings.Contains(line, " DO NOT EDIT.") {
			return true
		}
	}
	return false
}

func largestFileSummary(files gocloc.ClocFiles, limit int) []string {
	if len(files) < limit {
		limit = len(files)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, fmt.Sprintf("%s=%d", files[i].Name, files[i].Code))
	}
	return out
}
