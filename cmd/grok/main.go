package main

import (
	"os"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker"
)

// main simply calls the cli package's Cli() function
func main() {
	config := grokker.NewConfig()
	err := grokker.Cli(os.Args, config)
	Ck(err)
}
