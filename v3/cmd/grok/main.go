package main

import (
	"os"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/core"
)

// main simply calls the cli package's Cli() function
func main() {
	config := core.NewCliConfig()
	rc, err := core.Cli(os.Args[1:], config)
	Ck(err)
	os.Exit(rc)
}
