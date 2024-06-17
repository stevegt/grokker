package main

import (
	"os"

	"github.com/stevegt/aidda/x/x3"
	// . "github.com/stevegt/goadapt"
)

func main() {
	args := os.Args[1:]
	x3.Start(args...)
}
