package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"text/tabwriter"
	"time"

	"github.com/yokonao/cc-memory-view/internal/memory"
	"github.com/yokonao/cc-memory-view/internal/web"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

const usage = `Usage:
  cc-memory-view [serve] [flags]   browse memory in the browser
  cc-memory-view check [flags]     list audit candidates
  cc-memory-view --version
`

func main() {
	flag.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	showVersion := flag.Bool("version", false, "print the version")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	cmd, args := "serve", flag.Args()
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	var err error
	switch cmd {
	case "serve":
		err = serve(args)
	case "check":
		err = check(args)
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func staleFlag(fs *flag.FlagSet) *int {
	return fs.Int("stale-days", 90, "report memories not modified for this many days")
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:0", "listen address")
	noOpen := fs.Bool("no-open", false, "don't open the browser")
	staleDays := staleFlag(fs)
	_ = fs.Parse(args)

	root, err := memory.Root()
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	url := "http://" + ln.Addr().String() + "/"
	fmt.Println("Serving on", url)
	if !*noOpen {
		if err := openBrowser(url); err != nil {
			fmt.Fprintln(os.Stderr, "open browser:", err)
		}
	}
	s := &web.Server{Root: root, StaleAfter: days(*staleDays)}
	return http.Serve(ln, s.Handler())
}

func openBrowser(url string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	return exec.Command(name, url).Start()
}

func check(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print issues as JSON")
	staleDays := staleFlag(fs)
	_ = fs.Parse(args)

	root, err := memory.Root()
	if err != nil {
		return err
	}
	projects, err := memory.Load(root)
	if err != nil {
		return err
	}
	issues := memory.Check(projects, time.Now(), days(*staleDays))
	if *asJSON {
		if issues == nil {
			issues = []memory.Issue{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(issues)
	}
	return printIssues(os.Stdout, issues)
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

func days(n int) time.Duration {
	return time.Duration(n) * 24 * time.Hour
}
