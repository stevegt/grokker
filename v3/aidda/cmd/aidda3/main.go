package main

import (
	"os"

	"github.com/stevegt/aidda/x/x3"
	. "github.com/stevegt/goadapt"
)

// usage: go run main.go {subcommand}

func main() {
	Assert(len(os.Args) >= 2, "usage: go run main.go {subcommand ...}")
	for i := 1; i < len(os.Args); i++ {
		cmd := os.Args[1]
		err := x3.Do(cmd)
		Ck(err)
	}
}
