package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/yokonao/cc-memory-view/internal/audit"
)

const auditUsage = `Usage (run by the audit session):
  cc-memory-view audit update <token>   merge the state JSON on stdin
  cc-memory-view audit watch <token>    print replies from the web UI
`

func auditStore() (audit.Store, error) {
	dir, err := audit.Dir()
	if err != nil {
		return audit.Store{}, err
	}
	return audit.Store{Dir: filepath.Join(dir, "audits")}, nil
}

func auditCmd(args []string) error {
	if len(args) != 2 {
		fmt.Fprint(os.Stderr, auditUsage)
		os.Exit(2)
	}
	store, err := auditStore()
	if err != nil {
		return err
	}
	token := args[1]
	switch args[0] {
	case "update":
		if err := store.Update(token, os.Stdin); err != nil {
			return err
		}
		fmt.Println("Updated the web UI.")
		return nil
	case "watch":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return store.Watch(ctx, token, os.Stdout, time.Second)
	}
	fmt.Fprint(os.Stderr, auditUsage)
	os.Exit(2)
	return nil
}
