// Package budget keeps implementation LOC ceilings in a ledger file that only
// the main branch rewrites, so pull requests never edit budget numbers.
package budget

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/antonikliment/go-code-metrics/analysis"
)

// Root is the area key for files that no prefix claims.
const Root = "."

type Ledger struct {
	Global       int            `json:"global"`
	Areas        map[string]int `json:"areas"`
	ExcludeDirs  []string       `json:"exclude-dirs,omitempty"`
	IncludeTests bool           `json:"include-tests,omitempty"`
}

type Usage struct {
	Global int            `json:"global"`
	Areas  map[string]int `json:"areas"`
}

type Overrun struct {
	Area    string `json:"area"`
	Used    int    `json:"used"`
	Ceiling int    `json:"ceiling"`
}

func (o Overrun) Raise() int { return o.Used - o.Ceiling }

func Load(path string) (Ledger, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Ledger{}, err
	}
	var l Ledger
	if err := json.Unmarshal(data, &l); err != nil {
		return Ledger{}, fmt.Errorf("%s: %w", path, err)
	}
	return l, nil
}

func (l Ledger) Save(path string) error {
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// Measure counts implementation lines per area; files under no area prefix
// count toward Root, which only the global ceiling bounds unless listed.
func Measure(root string, l Ledger) (Usage, error) {
	tree, err := analysis.Analyze(analysis.Options{
		Root:             root,
		IncludeTests:     l.IncludeTests,
		ExcludeGenerated: true,
		ExcludeDirs:      l.ExcludeDirs,
	})
	if err != nil {
		return Usage{}, err
	}
	u := Usage{Global: tree.Code, Areas: map[string]int{}}
	for _, f := range analysis.Files(tree) {
		u.Areas[l.areaFor(filepath.ToSlash(f.Path))] += f.Code
	}
	return u, nil
}

func (l Ledger) areaFor(rel string) string {
	best := Root
	for prefix := range l.Areas {
		if (rel == prefix || strings.HasPrefix(rel, prefix+"/")) && (best == Root || len(prefix) > len(best)) {
			best = prefix
		}
	}
	return best
}

// Check lists every ceiling the usage exceeds, global first.
func Check(u Usage, l Ledger) []Overrun {
	var out []Overrun
	if u.Global > l.Global {
		out = append(out, Overrun{"global", u.Global, l.Global})
	}
	for _, area := range sortedKeys(l.Areas) {
		if u.Areas[area] > l.Areas[area] {
			out = append(out, Overrun{area, u.Areas[area], l.Areas[area]})
		}
	}
	return out
}

// Raise lifts each ceiling to the measured usage; ceilings never drop.
func Raise(l Ledger, u Usage) (Ledger, bool) {
	areas := make(map[string]int, len(l.Areas))
	changed := false
	for area, ceiling := range l.Areas {
		areas[area] = max(ceiling, u.Areas[area])
		changed = changed || areas[area] != ceiling
	}
	l.Areas = areas
	if u.Global > l.Global {
		l.Global, changed = u.Global, true
	}
	return l, changed
}

// RenderMarkdown writes a usage table; base is optional and adds a delta column.
func RenderMarkdown(w io.Writer, l Ledger, u Usage, base *Usage) error {
	row := func(name string, used, ceiling, before int) string {
		s := fmt.Sprintf("| `%s` | %d | %d | %d |", name, used, ceiling, ceiling-used)
		if base != nil {
			s += fmt.Sprintf(" %+d |", used-before)
		}
		return s + "\n"
	}
	header := "| area | used | ceiling | headroom |"
	sep := "|---|---:|---:|---:|"
	if base != nil {
		header, sep = header+" delta |", sep+"---:|"
	}
	var b strings.Builder
	b.WriteString(header + "\n" + sep + "\n")
	b.WriteString(row("global", u.Global, l.Global, baseOf(base, "")))
	for _, area := range sortedKeys(l.Areas) {
		b.WriteString(row(area, u.Areas[area], l.Areas[area], baseOf(base, area)))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func baseOf(base *Usage, area string) int {
	switch {
	case base == nil:
		return 0
	case area == "":
		return base.Global
	default:
		return base.Areas[area]
	}
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Seed sets every ceiling to current usage. With no areas it budgets each
// top-level directory that holds counted code.
func Seed(root string, l Ledger) (Ledger, error) {
	if len(l.Areas) == 0 {
		tree, err := analysis.Analyze(analysis.Options{
			Root: root, IncludeTests: l.IncludeTests, ExcludeGenerated: true, ExcludeDirs: l.ExcludeDirs,
		})
		if err != nil {
			return Ledger{}, err
		}
		l.Areas = map[string]int{}
		for _, f := range analysis.Files(tree) {
			if dir, _, ok := strings.Cut(filepath.ToSlash(f.Path), "/"); ok {
				l.Areas[dir] = 0
			}
		}
	}
	u, err := Measure(root, l)
	if err != nil {
		return Ledger{}, err
	}
	l.Global = u.Global
	for area := range l.Areas {
		l.Areas[area] = u.Areas[area]
	}
	return l, nil
}
