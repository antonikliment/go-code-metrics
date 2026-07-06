// Package analysis measures Go source trees.
package analysis

import (
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
		file, err := analyzeFile(path)
		if err != nil {
			return err
		}
		file.Name, file.Path = entry.Name(), rel
		parent := ensureDir(dirs, root, filepath.Dir(rel))
		parent.Children = append(parent.Children, file)
		return nil
	})
	if err != nil {
		return nil, err
	}
	aggregate(root)
	sortTree(root)
	return root, nil
}

func analyzeFile(path string) (*Node, error) {
	opts := gocloc.NewClocOptions()
	opts.IncludeLangs["Go"] = struct{}{}
	result, err := gocloc.NewProcessor(gocloc.NewDefinedLanguages(), opts).Analyze([]string{path})
	if err != nil {
		return nil, err
	}
	var code, comment, blank int
	for _, file := range result.Files {
		code, comment, blank = int(file.Code), int(file.Comments), int(file.Blanks)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	all := gocyclo.AnalyzeASTFile(file, fset, nil)
	stats := all.SortAndFilter(5, 0)
	node := &Node{IsFile: true, Code: code, Comment: comment, Blank: blank, Functions: len(all)}
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
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for _, line := range strings.SplitN(string(data), "\n", 12) {
		if generatedLine.MatchString(line) {
			return true
		}
	}
	return false
}

func ensureDir(dirs map[string]*Node, root *Node, dir string) *Node {
	if dir == "." || dir == "" {
		return root
	}
	if node := dirs[dir]; node != nil {
		return node
	}
	parent := ensureDir(dirs, root, filepath.Dir(dir))
	node := &Node{Name: filepath.Base(dir), Path: dir}
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
