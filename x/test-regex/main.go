package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	// . "github.com/stevegt/goadapt"
)

/*

Test a regex against a file, printing the match if it matches, "no match" if it doesn't.

Usage:
	go run main.go <filename> <regex> <matchnumber>

*/

func main() {
	// get the filename, regex, and match number from command line arguments
	if len(os.Args) < 4 {
		fmt.Println("Usage: go run main.go <filename> <regex> <matchnumber>")
		os.Exit(1)
	}
	filename := os.Args[1]
	regexStr := os.Args[2]
	matchNumberStr := os.Args[3]

	// convert match number to integer
	var matchNumber int
	fmt.Sscanf(matchNumberStr, "%d", &matchNumber)
	if matchNumber < 0 {
		fmt.Println("Match number must be a non-negative integer")
		os.Exit(1)
	}

	// read the file
	buf := bytes.Buffer{}
	file, err := os.Open(filename)
	if err != nil {
		fmt.Println("Error opening file:", err)
		os.Exit(1)
	}
	defer file.Close()
	_, err = io.Copy(&buf, file)
	if err != nil {
		fmt.Println("Error reading file:", err)
		os.Exit(1)
	}

	// compile the regex
	regex, err := regexp.Compile(regexStr)
	if err != nil {
		fmt.Println("Error compiling regex:", err)
		os.Exit(1)
	}

	// get the submatches
	txt := buf.String()
	matches := regex.FindStringSubmatch(txt)
	if matches == nil {
		fmt.Println("no match")
		os.Exit(0)
	}

	// print the match
	output := matches[matchNumber]
	fmt.Print(output)
}
