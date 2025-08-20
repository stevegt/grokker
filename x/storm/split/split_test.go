package split

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

func TestSplit(t *testing.T) {
	// take input from stdin XXX needs to be a file in ./testdata
	input, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Error reading input: %v\n", err)
		os.Exit(1)
	}
	// Parse the storm file from the input
	roundTrips, err := Parse(bytes.NewReader(input))
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: Error parsing storm file: %v\n", err)
		os.Exit(1)
	}
	// Print each round trip
	for _, rt := range roundTrips {
		fmt.Println(rt)
	}
	// Exit with success
	os.Exit(0)
}
