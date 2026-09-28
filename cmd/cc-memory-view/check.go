package main

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/yokonao/cc-memory-view/internal/memory"
)

func checkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check",
		Short: "List audit candidates",
		Args:  cobra.NoArgs,
	}
	asJSON := cmd.Flags().Bool("json", false, "print issues as JSON")
	staleDays := staleDaysFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return check(cmd.OutOrStdout(), *asJSON, *staleDays)
	}
	return cmd
}

func check(w io.Writer, asJSON bool, staleDays int) error {
	root, err := memory.Root()
	if err != nil {
		return err
	}
	projects, err := memory.Load(root)
	if err != nil {
		return err
	}
	issues := memory.Check(projects, time.Now(), days(staleDays))
	if asJSON {
		if issues == nil {
			issues = []memory.Issue{}
		}
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(issues)
	}
	return printIssues(w, issues)
}

func printIssues(w io.Writer, issues []memory.Issue) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, i := range issues {
		file := filepath.Base(i.File)
		if file == "memory" {
			file = "-"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", i.Kind, i.Project, file, i.Detail)
	}
	return tw.Flush()
}
