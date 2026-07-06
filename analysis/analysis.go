// Package analysis measures Go source trees.
package analysis

import (
	"bufio"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fzipp/gocyclo"
	"github.com/hhatto/gocloc"
)

type Node struct {
	Name          string     `json:"name"`
	Path          string     `json:"path"`
	IsFile        bool       `json:"isFile"`
	Code          int        `json:"code"`
	Comment       int        `json:"comment"`
	Blank         int        `json:"blank"`
	Complexity    int        `json:"complexity"`
	MaxComplexity int        `json:"maxComplexity"`
	Functions     int        `json:"functions"`
	Hotspots      []FuncStat `json:"hotspots,omitempty"`
	Children      []*Node    `json:"children,omitempty"`
	Warnings      []string   `json:"warnings,omitempty"`
}

type FuncStat struct {
	Name       string `json:"name"`
	Complexity int    `json:"complexity"`
	Line       int    `json:"line"`
}

type Options struct {
	Root             string
	IncludeTests     bool
	ExcludeGenerated bool
	ExcludeDirs      []string
	HotspotLimit     int
	ContinueOnError  bool
}

func Analyze(opt Options) (*Node, error) {
	if opt.Root == "" {
		opt.Root = "."
	}
	abs, err := filepath.Abs(opt.Root)
	if err != nil {
		return nil, err
	}
	root := &Node{Name: filepath.Base(abs), Path: "."}
	dirs := map[string]*Node{".": root}
	type candidate struct{ path, rel, name string }
	var candidates []candidate
	err = filepath.WalkDir(opt.Root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(opt.Root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if rel != "." && excludedDir(rel, entry.Name(), opt.ExcludeDirs) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") || (!opt.IncludeTests && strings.HasSuffix(entry.Name(), "_test.go")) {
			return nil
		}
		if opt.ExcludeGenerated && generated(path) {
			return nil
		}
		candidates = append(candidates, candidate{path: path, rel: rel, name: entry.Name()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(candidates))
	for i, file := range candidates {
		paths[i] = file.path
	}
	counts, err := countFiles(paths)
	if err != nil {
		return nil, err
	}
	limit := opt.HotspotLimit
	if limit == 0 {
		limit = 5
	}
	for _, candidate := range candidates {
		file, err := analyzeFile(candidate.path, counts[candidate.path], limit)
		if err != nil && !opt.ContinueOnError {
			return nil, err
		}
		if err != nil {
			root.Warnings = append(root.Warnings, err.Error())
		}
		file.Name, file.Path = candidate.name, filepath.ToSlash(candidate.rel)
		parent := ensureDir(dirs, root, filepath.Dir(candidate.rel))
		parent.Children = append(parent.Children, file)
	}
	aggregate(root)
	sortTree(root)
	return root, nil
}

func countFiles(paths []string) (map[string]*gocloc.ClocFile, error) {
	opts := gocloc.NewClocOptions()
	opts.IncludeLangs["Go"] = struct{}{}
	result, err := gocloc.NewProcessor(gocloc.NewDefinedLanguages(), opts).Analyze(paths)
	if err != nil {
		return nil, err
	}
	return result.Files, nil
}

func analyzeFile(path string, count *gocloc.ClocFile, hotspotLimit int) (*Node, error) {
	if count == nil {
		return nil, fmt.Errorf("gocloc did not return metrics for %s", path)
	}
	node := &Node{IsFile: true, Code: int(count.Code), Comment: int(count.Comments), Blank: int(count.Blanks)}
	src, err := os.ReadFile(path)
	if err != nil {
		return node, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	if err != nil {
		return node, err
	}
	all := gocyclo.AnalyzeASTFile(file, fset, nil)
	stats := all.SortAndFilter(hotspotLimit, 0)
	node.Functions = len(all)
	for _, stat := range all {
		node.Complexity += stat.Complexity
		if stat.Complexity > node.MaxComplexity {
			node.MaxComplexity = stat.Complexity
		}
	}
	for _, stat := range stats {
		node.Hotspots = append(node.Hotspots, FuncStat{Name: stat.FuncName, Complexity: stat.Complexity, Line: stat.Pos.Line})
	}
	return node, nil
}

func excludedDir(rel, name string, dirs []string) bool {
	if strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "node_modules" {
		return true
	}
	rel = filepath.ToSlash(rel)
	for _, dir := range dirs {
		if strings.Trim(strings.TrimSpace(filepath.ToSlash(dir)), "/") == rel {
			return true
		}
	}
	return false
}

var generatedLine = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

func generated(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for line := 0; line < 12 && scanner.Scan(); line++ {
		if generatedLine.MatchString(scanner.Text()) {
			return true
		}
	}
	return false
}

// Files returns every file node in tree order.
func Files(node *Node) []*Node {
	if node.IsFile {
		return []*Node{node}
	}
	var files []*Node
	for _, child := range node.Children {
		files = append(files, Files(child)...)
	}
	return files
}

func ensureDir(dirs map[string]*Node, root *Node, dir string) *Node {
	if dir == "." || dir == "" {
		return root
	}
	if node := dirs[dir]; node != nil {
		return node
	}
	parent := ensureDir(dirs, root, filepath.Dir(dir))
	node := &Node{Name: filepath.Base(dir), Path: filepath.ToSlash(dir)}
	dirs[dir] = node
	parent.Children = append(parent.Children, node)
	return node
}

func aggregate(node *Node) {
	for _, child := range node.Children {
		aggregate(child)
		node.Code += child.Code
		node.Comment += child.Comment
		node.Blank += child.Blank
		node.Complexity += child.Complexity
		node.Functions += child.Functions
		if child.MaxComplexity > node.MaxComplexity {
			node.MaxComplexity = child.MaxComplexity
		}
	}
}

func sortTree(node *Node) {
	sort.SliceStable(node.Children, func(i, j int) bool { return node.Children[i].Code > node.Children[j].Code })
	for _, child := range node.Children {
		sortTree(child)
	}
}
