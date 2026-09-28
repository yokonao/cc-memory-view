package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/yokonao/cc-memory-view/internal/audit"
)

func auditCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Used by the audit session to talk to the web UI",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "update <token>",
			Short: "Merge the state JSON on stdin",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				store, err := auditStore()
				if err != nil {
					return err
				}
				if err := store.Update(args[0], cmd.InOrStdin()); err != nil {
					return err
				}
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "Updated the web UI.")
				return err
			},
		},
		&cobra.Command{
			Use:   "watch <token>",
			Short: "Print replies from the web UI",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				store, err := auditStore()
				if err != nil {
					return err
				}
				return store.Watch(cmd.Context(), args[0], cmd.OutOrStdout(), time.Second)
			},
		},
	)
	return cmd
}

func auditStore() (audit.Store, error) {
	dir, err := audit.Dir()
	if err != nil {
		return audit.Store{}, err
	}
	return audit.Store{Dir: filepath.Join(dir, "audits")}, nil
}
