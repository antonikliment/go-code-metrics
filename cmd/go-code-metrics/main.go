// Command go-code-metrics manages project setup for Go code metrics tooling.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/antonikliment/go-code-metrics/install"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] != "install" {
		return fmt.Errorf("usage: go-code-metrics install [-root directory]")
	}
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	root := flags.String("root", ".", "project directory")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("usage: go-code-metrics install [-root directory]")
	}
	results, err := install.Ensure(*root)
	if err != nil {
		return fmt.Errorf("install lint configuration: %w", err)
	}
	for _, result := range results {
		status := "exists"
		if result.Created {
			status = "created"
		}
		fmt.Fprintf(out, "%s: %s\n", status, result.Path)
	}
	return nil
}
