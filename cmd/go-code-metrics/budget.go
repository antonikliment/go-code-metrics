package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/antonikliment/go-code-metrics/budget"
)

const budgetUsage = "usage: go-code-metrics budget init|report|check|raise [-root dir] [-ledger file] [-base-root dir] [-areas a,b] [-exclude d,e]"

var errOverBudget = errors.New("over budget")

// runBudget: report prints the table; check also fails on any overrun;
// raise rewrites the ledger so every ceiling covers the measured tree.
func runBudget(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(budgetUsage)
	}
	flags := flag.NewFlagSet("budget", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", ".", "tree to measure")
	ledgerPath := flags.String("ledger", "", "ledger file (default <root>/.loc-budgets.json)")
	baseRoot := flags.String("base-root", "", "base tree for a delta column")
	areas := flags.String("areas", "", "init: comma-separated area prefixes (default: top-level dirs)")
	exclude := flags.String("exclude", "vendor,.git,node_modules", "init: comma-separated excluded dirs")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return errors.New(budgetUsage)
	}
	if *ledgerPath == "" {
		*ledgerPath = filepath.Join(*root, ".loc-budgets.json")
	}
	if args[0] == "init" {
		return initLedger(out, *root, *ledgerPath, *areas, *exclude)
	}
	l, err := budget.Load(*ledgerPath)
	if err != nil {
		return err
	}
	u, err := budget.Measure(*root, l)
	if err != nil {
		return err
	}
	switch args[0] {
	case "report":
		return report(out, l, u, *baseRoot)
	case "check":
		if err := report(out, l, u, *baseRoot); err != nil {
			return err
		}
		return checkOverruns(out, budget.Check(u, l))
	case "raise":
		raised, changed := budget.Raise(l, u)
		if !changed {
			_, err := fmt.Fprintln(out, "ceilings already cover the tree")
			return err
		}
		return raised.Save(*ledgerPath)
	}
	return errors.New(budgetUsage)
}

func report(out io.Writer, l budget.Ledger, u budget.Usage, baseRoot string) error {
	if baseRoot == "" {
		return budget.RenderMarkdown(out, l, u, nil)
	}
	base, err := budget.Measure(baseRoot, l)
	if err != nil {
		return fmt.Errorf("measure base: %w", err)
	}
	return budget.RenderMarkdown(out, l, u, &base)
}

func checkOverruns(out io.Writer, over []budget.Overrun) error {
	if len(over) == 0 {
		return nil
	}
	parts := make([]string, len(over))
	for i, o := range over {
		parts[i] = fmt.Sprintf("%s +%d", o.Area, o.Raise())
	}
	if _, err := fmt.Fprintf(out, "\nraise needed: %s\n", strings.Join(parts, ", ")); err != nil {
		return err
	}
	return errOverBudget
}

func initLedger(out io.Writer, root, path, areas, exclude string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s exists; edit it or run raise", path)
	}
	l := budget.Ledger{Areas: map[string]int{}, ExcludeDirs: splitList(exclude)}
	for _, a := range splitList(areas) {
		l.Areas[a] = 0
	}
	l, err := budget.Seed(root, l)
	if err != nil {
		return err
	}
	if err := l.Save(path); err != nil {
		return err
	}
	return budget.RenderMarkdown(out, l, budget.Usage{Global: l.Global, Areas: l.Areas}, nil)
}

func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
