package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode"
)

type State int

const (
	stateBody State = iota
	stateReferences
	stateReasoning
	stateHR
)

// returns true if the line starts with "- [" followed by a digit.
func startsWithDashDigit(line string) bool {
	if !strings.HasPrefix(line, "- [") {
		return false
	}
	// Make sure there is a character after "- ["
	if len(line) < 4 {
		return false
	}
	// Check if the first character after "- [" is a digit
	return unicode.IsDigit(rune(line[3]))
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	state := stateBody

	for scanner.Scan() {
		line := scanner.Text()
		insert := ""
		skip := false

		switch state {
		case stateBody:
			if strings.HasPrefix(line, "## References") {
				state = stateReferences
			} else if startsWithDashDigit(line) {
				// Insert missing "## References" header before the line.
				insert = "## References\n\n"
				state = stateReferences
			} else if strings.HasPrefix(line, "## Reasoning") {
				state = stateReasoning
			} else if strings.HasPrefix(line, "---") {
				state = stateHR
			}
		case stateReferences:
			// If line starts with "- [<digit>]" or "## References" or is blank, stay in references.
			if startsWithDashDigit(line) || strings.TrimSpace(line) == "" {
				// remain in stateReferences.
			} else if strings.HasPrefix(line, "## References") {
				// remove "## References" header if it is a duplicate.
				skip = true
				// remain in stateReferences.
			} else if strings.HasPrefix(line, "## Reasoning") {
				state = stateReasoning
			} else if strings.HasPrefix(line, "---") {
				state = stateHR
			} else {
				// For any other non-blank line, insert missing "## Reasoning" header.
				insert = "## Reasoning\n\n"
				state = stateReasoning
			}
		case stateReasoning:
			if strings.HasPrefix(line, "---") {
				state = stateHR
			}
		case stateHR:
			// According to the rules, any state hr transitions immediately to body.
			state = stateBody
			// No other processing is needed.
		}

		// Print any inserted heading.
		if insert != "" {
			fmt.Print(insert)
		}

		// Print the current line.
		if !skip {
			fmt.Println(line)
		}

		// After printing the line, if the current line triggered hr, reset state to body.
		if strings.HasPrefix(line, "---") {
			state = stateBody
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
		os.Exit(1)
	}
}
