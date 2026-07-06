// Package comparison measures Go metric changes between Git revisions.
package comparison

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/antonikliment/go-code-metrics/analysis"
)

type Options struct {
	Root             string
	Base             string
	IncludeTests     bool
	ExcludeGenerated bool
	ExcludeDirs      []string
	ContinueOnError  bool
	HotspotLimit     int
}

type Result struct {
	Base              string       `json:"base"`
	MergeBase         string       `json:"mergeBase"`
	LinesAdded        int          `json:"linesAdded"`
	LinesDeleted      int          `json:"linesDeleted"`
	CodeBefore        int          `json:"codeBefore"`
	CodeAfter         int          `json:"codeAfter"`
	ComplexityBefore  int          `json:"complexityBefore"`
	ComplexityAfter   int          `json:"complexityAfter"`
	ComplexityAdded   int          `json:"complexityAdded"`
	ComplexityRemoved int          `json:"complexityRemoved"`
	Files             []FileChange `json:"files"`
	Warnings          []string     `json:"warnings,omitempty"`
}

type FileChange struct {
	Path              string           `json:"path"`
	OldPath           string           `json:"oldPath,omitempty"`
	Status            string           `json:"status"`
	LinesAdded        int              `json:"linesAdded"`
	LinesDeleted      int              `json:"linesDeleted"`
	CodeBefore        int              `json:"codeBefore"`
	CodeAfter         int              `json:"codeAfter"`
	ComplexityBefore  int              `json:"complexityBefore"`
	ComplexityAfter   int              `json:"complexityAfter"`
	ComplexityAdded   int              `json:"complexityAdded"`
	ComplexityRemoved int              `json:"complexityRemoved"`
	Functions         []FunctionChange `json:"functions,omitempty"`
}

type FunctionChange struct {
	Name   string `json:"name"`
	Before int    `json:"before"`
	After  int    `json:"after"`
	Delta  int    `json:"delta"`
	Status string `json:"status"`
	Line   int    `json:"line,omitempty"`
}

type gitChange struct {
	status, oldRepo, newRepo, oldPath, newPath string
	added, deleted                             int
}

func Analyze(opt Options) (*Result, error) {
	if opt.Root == "" {
		opt.Root = "."
	}
	if opt.Base == "" {
		opt.Base = "main"
	}
	root, scope, err := repositoryRoot(opt.Root)
	if err != nil {
		return nil, err
	}
	mergeBase, err := git(root, "merge-base", opt.Base, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("resolve merge base for %q (fetch the base branch and full history if needed): %w", opt.Base, err)
	}
	mergeBase = strings.TrimSpace(mergeBase)
	changes, err := changedFiles(root, scope, mergeBase)
	if err != nil {
		return nil, err
	}
	baseDir, headDir, cleanup, err := materializeSnapshots(root, mergeBase, changes)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	baseTree, headTree, err := analyzeSnapshots(baseDir, headDir, opt)
	if err != nil {
		return nil, err
	}
	return buildResult(opt, mergeBase, changes, baseTree, headTree), nil
}

func materializeSnapshots(root, mergeBase string, changes []gitChange) (string, string, func(), error) {
	baseDir, err := os.MkdirTemp("", "go-code-metrics-base-")
	if err != nil {
		return "", "", func() {}, err
	}
	headDir, err := os.MkdirTemp("", "go-code-metrics-head-")
	if err != nil {
		os.RemoveAll(baseDir)
		return "", "", func() {}, err
	}
	cleanup := func() { os.RemoveAll(baseDir); os.RemoveAll(headDir) }
	for _, change := range changes {
		if change.oldRepo != "" {
			data, err := gitBytes(root, "show", mergeBase+":"+filepath.ToSlash(change.oldRepo))
			if err != nil {
				cleanup()
				return "", "", func() {}, fmt.Errorf("read base file %s: %w", change.oldRepo, err)
			}
			if err := writeFile(baseDir, change.oldPath, data); err != nil {
				cleanup()
				return "", "", func() {}, err
			}
		}
		if change.newRepo != "" {
			data, err := os.ReadFile(filepath.Join(root, change.newRepo))
			if err != nil {
				cleanup()
				return "", "", func() {}, fmt.Errorf("read changed file %s: %w", change.newRepo, err)
			}
			if err := writeFile(headDir, change.newPath, data); err != nil {
				cleanup()
				return "", "", func() {}, err
			}
		}
	}
	return baseDir, headDir, cleanup, nil
}

func analyzeSnapshots(baseDir, headDir string, opt Options) (*analysis.Node, *analysis.Node, error) {
	analyzeOptions := func(root string) analysis.Options {
		return analysis.Options{Root: root, IncludeTests: opt.IncludeTests, ExcludeGenerated: opt.ExcludeGenerated,
			ExcludeDirs: opt.ExcludeDirs, HotspotLimit: -1, ContinueOnError: opt.ContinueOnError}
	}
	baseTree, err := analysis.Analyze(analyzeOptions(baseDir))
	if err != nil {
		return nil, nil, fmt.Errorf("analyze base: %w", err)
	}
	headTree, err := analysis.Analyze(analyzeOptions(headDir))
	if err != nil {
		return nil, nil, fmt.Errorf("analyze working tree: %w", err)
	}
	return baseTree, headTree, nil
}

func buildResult(opt Options, mergeBase string, changes []gitChange, baseTree, headTree *analysis.Node) *Result {
	baseFiles, headFiles := fileMap(baseTree), fileMap(headTree)
	result := &Result{Base: opt.Base, MergeBase: mergeBase, Warnings: append(baseTree.Warnings, headTree.Warnings...)}
	for _, change := range changes {
		before, after := baseFiles[filepath.ToSlash(change.oldPath)], headFiles[filepath.ToSlash(change.newPath)]
		if before == nil && after == nil {
			continue
		}
		file := compareFile(change, before, after)
		limit := opt.HotspotLimit
		if limit == 0 {
			limit = 5
		}
		if limit >= 0 && limit < len(file.Functions) {
			file.Functions = file.Functions[:limit]
		}
		result.Files = append(result.Files, file)
		result.LinesAdded += file.LinesAdded
		result.LinesDeleted += file.LinesDeleted
		result.CodeBefore += file.CodeBefore
		result.CodeAfter += file.CodeAfter
		result.ComplexityBefore += file.ComplexityBefore
		result.ComplexityAfter += file.ComplexityAfter
		result.ComplexityAdded += file.ComplexityAdded
		result.ComplexityRemoved += file.ComplexityRemoved
	}
	sort.SliceStable(result.Files, func(i, j int) bool {
		if result.Files[i].ComplexityAdded != result.Files[j].ComplexityAdded {
			return result.Files[i].ComplexityAdded > result.Files[j].ComplexityAdded
		}
		if result.Files[i].LinesAdded != result.Files[j].LinesAdded {
			return result.Files[i].LinesAdded > result.Files[j].LinesAdded
		}
		return result.Files[i].Path < result.Files[j].Path
	})
	return result
}

func compareFile(change gitChange, before, after *analysis.Node) FileChange {
	file := FileChange{Path: filepath.ToSlash(change.newPath), OldPath: filepath.ToSlash(change.oldPath), Status: change.status,
		LinesAdded: change.added, LinesDeleted: change.deleted}
	if file.Path == "" {
		file.Path = file.OldPath
	}
	if file.Path == file.OldPath {
		file.OldPath = ""
	}
	if before != nil {
		file.CodeBefore, file.ComplexityBefore = before.Code, before.Complexity
	}
	if after != nil {
		file.CodeAfter, file.ComplexityAfter = after.Code, after.Complexity
	}
	oldFunctions, newFunctions := functionMap(before), functionMap(after)
	for name, current := range newFunctions {
		previous, found := oldFunctions[name]
		change := FunctionChange{Name: name, After: current.Complexity, Line: current.Line, Status: "added"}
		if found {
			change.Before, change.Status = previous.Complexity, "modified"
			delete(oldFunctions, name)
		}
		change.Delta = change.After - change.Before
		if change.Delta == 0 {
			continue
		}
		if change.Delta > 0 {
			file.ComplexityAdded += change.Delta
		} else {
			file.ComplexityRemoved -= change.Delta
		}
		file.Functions = append(file.Functions, change)
	}
	for name, previous := range oldFunctions {
		file.ComplexityRemoved += previous.Complexity
		file.Functions = append(file.Functions, FunctionChange{Name: name, Before: previous.Complexity, Delta: -previous.Complexity, Status: "deleted"})
	}
	sort.SliceStable(file.Functions, func(i, j int) bool {
		if file.Functions[i].Delta != file.Functions[j].Delta {
			return file.Functions[i].Delta > file.Functions[j].Delta
		}
		return file.Functions[i].Name < file.Functions[j].Name
	})
	return file
}

func functionMap(node *analysis.Node) map[string]analysis.FuncStat {
	functions := map[string]analysis.FuncStat{}
	if node != nil {
		for _, function := range node.Hotspots {
			functions[function.Name] = function
		}
	}
	return functions
}

func fileMap(root *analysis.Node) map[string]*analysis.Node {
	files := map[string]*analysis.Node{}
	for _, file := range analysis.Files(root) {
		files[file.Path] = file
	}
	return files
}

func repositoryRoot(start string) (root, scope string, err error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", "", err
	}
	out, err := git(abs, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", fmt.Errorf("find Git repository: %w", err)
	}
	root = strings.TrimSpace(out)
	scope, err = filepath.Rel(root, abs)
	return root, scope, err
}

func changedFiles(root, scope, mergeBase string) ([]gitChange, error) {
	out, err := gitBytes(root, "diff", "--name-status", "-z", "--find-renames", mergeBase)
	if err != nil {
		return nil, fmt.Errorf("list changed files: %w", err)
	}
	changes, err := parseNameStatus(out, scope)
	if err != nil {
		return nil, err
	}
	changes, err = appendUntracked(root, scope, changes)
	if err != nil {
		return nil, err
	}
	if err := addNumStats(root, mergeBase, changes); err != nil {
		return nil, err
	}
	return changes, nil
}

func appendUntracked(root, scope string, changes []gitChange) ([]gitChange, error) {
	untracked, err := gitBytes(root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for _, path := range splitZero(untracked) {
		if rel, ok := scopedGoPath(path, scope); ok {
			data, err := os.ReadFile(filepath.Join(root, path))
			if err != nil {
				return nil, err
			}
			changes = append(changes, gitChange{status: "added", newRepo: path, newPath: rel, added: lineCount(data)})
		}
	}
	return changes, nil
}

func addNumStats(root, mergeBase string, changes []gitChange) error {
	for i := range changes {
		if changes[i].status == "added" && changes[i].added > 0 {
			continue
		}
		args := []string{"diff", "--numstat", "--find-renames", mergeBase, "--"}
		if changes[i].oldRepo != "" {
			args = append(args, changes[i].oldRepo)
		}
		if changes[i].newRepo != "" && changes[i].newRepo != changes[i].oldRepo {
			args = append(args, changes[i].newRepo)
		}
		numbers, err := git(root, args...)
		if err != nil {
			return err
		}
		for _, line := range strings.Split(strings.TrimSpace(numbers), "\n") {
			parts := strings.SplitN(line, "\t", 3)
			if len(parts) >= 2 {
				changes[i].added, _ = strconv.Atoi(parts[0])
				changes[i].deleted, _ = strconv.Atoi(parts[1])
				break
			}
		}
	}
	return nil
}

func lineCount(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	lines := bytes.Count(data, []byte{'\n'})
	if data[len(data)-1] != '\n' {
		lines++
	}
	return lines
}

func parseNameStatus(data []byte, scope string) ([]gitChange, error) {
	fields := splitZero(data)
	var changes []gitChange
	for i := 0; i < len(fields); {
		code := fields[i]
		i++
		if code == "" || i >= len(fields) {
			continue
		}
		var oldRepo, newRepo string
		switch code[0] {
		case 'R', 'C':
			if i+1 >= len(fields) {
				return nil, fmt.Errorf("invalid Git rename output")
			}
			oldRepo, newRepo = fields[i], fields[i+1]
			i += 2
		case 'D':
			oldRepo = fields[i]
			i++
		default:
			newRepo = fields[i]
			oldRepo = newRepo
			i++
			if code[0] == 'A' {
				oldRepo = ""
			}
		}
		oldPath, oldOK := scopedGoPath(oldRepo, scope)
		newPath, newOK := scopedGoPath(newRepo, scope)
		if !oldOK && !newOK {
			continue
		}
		status := map[byte]string{'A': "added", 'D': "deleted", 'R': "renamed", 'C': "copied"}[code[0]]
		if status == "" {
			status = "modified"
		}
		changes = append(changes, gitChange{status: status, oldRepo: oldRepo, newRepo: newRepo, oldPath: oldPath, newPath: newPath})
	}
	return changes, nil
}

func scopedGoPath(path, scope string) (string, bool) {
	if path == "" || !strings.HasSuffix(path, ".go") {
		return "", false
	}
	if scope == "." {
		return filepath.FromSlash(path), true
	}
	rel, err := filepath.Rel(scope, filepath.FromSlash(path))
	return rel, err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func splitZero(data []byte) []string {
	parts := bytes.Split(data, []byte{0})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 0 {
			out = append(out, string(part))
		}
	}
	return out
}

func writeFile(root, path string, data []byte) error {
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o600)
}

func git(root string, args ...string) (string, error) {
	data, err := gitBytes(root, args...)
	return string(data), err
}

func gitBytes(root string, args ...string) ([]byte, error) {
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	data, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(data)))
	}
	return data, nil
}
