package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:               "cc-memory-view",
		Short:             "Browse and audit Claude Code's auto memory",
		Version:           version,
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.AddCommand(serveCmd(), checkCmd(), auditCmd())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := root.ExecuteContext(ctx); err != nil {
		stop()
		os.Exit(1)
	}
}

func staleDaysFlag(cmd *cobra.Command) *int {
	return cmd.Flags().Int("stale-days", 90, "report memories not modified for this many days")
}

func days(n int) time.Duration {
	return time.Duration(n) * 24 * time.Hour
}
