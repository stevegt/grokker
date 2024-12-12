package main

import (
	"github.com/stevegt/grokker/v3/cli"
	"os"

	. "github.com/stevegt/goadapt"
)

// main simply calls the cli package's Cli() function
func main() {
	config := cli.NewCliConfig()
	rc, err := cli.Cli(os.Args[1:], config)
	Ck(err)
	os.Exit(rc)
}
