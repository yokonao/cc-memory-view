package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/yokonao/cc-memory-view/internal/audit"
	"github.com/yokonao/cc-memory-view/internal/memory"
	"github.com/yokonao/cc-memory-view/internal/web"
)

func serveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Browse memory in the browser",
		Args:  cobra.NoArgs,
	}
	addr := cmd.Flags().String("addr", "127.0.0.1:0", "listen address: host:port or unix:///absolute/path")
	noOpen := cmd.Flags().Bool("no-open", false, "don't open the browser")
	staleDays := staleDaysFlag(cmd)
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return serve(cmd.Context(), *addr, !*noOpen, *staleDays)
	}
	return cmd
}

func serve(ctx context.Context, addr string, open bool, staleDays int) error {
	configDir, err := memory.ConfigDir()
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	auditDir, err := audit.Dir()
	if err != nil {
		return err
	}
	ln, err := web.Listen(addr)
	if err != nil {
		return err
	}
	if ln.Addr().Network() == "unix" {
		fmt.Println("Serving on unix://" + ln.Addr().String())
	} else {
		url := "http://" + ln.Addr().String() + "/"
		fmt.Println("Serving on", url)
		if open {
			if err := openBrowser(url); err != nil {
				fmt.Fprintln(os.Stderr, "open browser:", err)
			}
		}
	}

	s := &web.Server{
		Root:       filepath.Join(configDir, "projects"),
		StaleAfter: days(staleDays),
		Audits:     audit.Store{Dir: filepath.Join(auditDir, "audits")},
		StartAudit: func(ctx context.Context, token, project string) (string, error) {
			p, err := findProject(filepath.Join(configDir, "projects"), project)
			if err != nil {
				return "", err
			}
			return audit.Start(ctx, audit.Params{Dir: p.Path, ConfigDir: configDir, MemoryDir: filepath.Join(p.Dir, "memory"), Exe: exe, Token: token})
		},
	}
	srv := &http.Server{Handler: s.Handler()}
	go func() {
		<-ctx.Done()
		// Closing the listener also removes the Unix socket.
		_ = srv.Close()
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func findProject(root, name string) (*memory.Project, error) {
	projects, err := memory.Load(root)
	if err != nil {
		return nil, err
	}
	for _, p := range projects {
		if p.Name() == name && p.Path != "" {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no project directory known for %s", name)
}

func openBrowser(url string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	return exec.Command(name, url).Start()
}
