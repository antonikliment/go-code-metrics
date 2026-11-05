package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/antonikliment/go-code-metrics/budget"
)

const budgetUsage = "usage: go-code-metrics budget report|check|raise [-root dir] [-ledger file] [-base-root dir]"

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
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return errors.New(budgetUsage)
	}
	if *ledgerPath == "" {
		*ledgerPath = filepath.Join(*root, ".loc-budgets.json")
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
