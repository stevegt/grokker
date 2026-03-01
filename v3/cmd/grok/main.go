package main

import (
	"fmt"
	"github.com/stevegt/grokker/v3/cli"
	"os"
)

// main simply calls the cli package's Cli() function
func main() {
	config := cli.NewCliConfig()
	rc, err := cli.Cli(os.Args[1:], config)
	if err != nil {
		// Print a concise error message without a stack trace.
		fmt.Fprintln(os.Stderr, err)
		if rc == 0 {
			rc = 1
		}
	}
	os.Exit(rc)
}
