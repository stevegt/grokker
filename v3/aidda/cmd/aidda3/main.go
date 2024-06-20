package main

import (
	"os"

	"github.com/stevegt/aidda/x/x3"
	. "github.com/stevegt/goadapt"
)

// usage: go run main.go {subcommand}

func main() {
	args := os.Args[1:]
	err := x3.Do(args...)
	Ck(err)
}
