package main

import (
	"fmt"
	"os"

	"github.com/aixgo-dev/sync/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version.Version)
		return
	}
	fmt.Fprintf(os.Stderr, "aixgo-sync %s — scaffold; see docs/PRD.md\n", version.Version)
	fmt.Fprintf(os.Stderr, "usage: aixgo-sync version\n")
	os.Exit(0)
}
