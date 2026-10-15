package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"

	"github.com/spf13/cobra"
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
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		return serve(cmd.Context(), *addr, !*noOpen)
	}
	return cmd
}

func serve(ctx context.Context, addr string, open bool) error {
	root, err := memory.Root()
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

	s := &web.Server{Root: root}
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

func openBrowser(url string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	return exec.Command(name, url).Start()
}
