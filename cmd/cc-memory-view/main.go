package main

import (
	"flag"
	"fmt"
	"os"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}
	fmt.Fprintln(os.Stderr, "not implemented yet")
	os.Exit(1)
}
