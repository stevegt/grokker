package split

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	. "github.com/stevegt/goadapt"
)

// RoundTrip represents one complete exchange in a storm file,
// containing the query, the response, the references and the reasoning.
type RoundTrip struct {
	Query      string
	Response   string
	References string
	Reasoning  string
}

// Parse reads a storm file from the provided io.Reader and returns
// a slice of RoundTrip structs. A storm file is expected to consist of
// one or more round-trip blocks separated by a line matching "^---$".
// Each block is further parsed using heuristics:
//   - The first occurrence of text in double asterisks (**) is taken as the query.
//   - The text between the query and a "## References" marker is taken as the response.
//   - The text between "## References" and "## Reasoning" is taken as the references.
//   - The text after "## Reasoning" is taken as the reasoning.
//
// If any of these markers are missing, the corresponding field is set to an empty string.
func Parse(r io.Reader) ([]RoundTrip, error) {
	// Read entire file contents
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Normalize line endings and trim surrounding whitespace.
	content := string(data)
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSpace(content)

	// Split the file into blocks using lines that exactly match "---"
	// The regex works in multiline mode.
	splitRegex := regexp.MustCompile(`(?m)^---[ \t]*$`)
	blocks := splitRegex.Split(content, -1)
	Fpf(os.Stderr, "INFO: Split storm file into %d blocks\n", len(blocks))

	// if the last block is empty, remove it
	if len(blocks) > 0 && strings.TrimSpace(blocks[len(blocks)-1]) == "" {
		Fpf(os.Stderr, "INFO: Removing empty last block\n")
		blocks = blocks[:len(blocks)-1]
	}

	var rounds []RoundTrip
	// Regular expression to capture the first multiline bold text as the query.
	boldRegex := regexp.MustCompile(`^\*\*((.|\n)*?)\*\*`)
	// Regular expression to capture the first multiline non-whitespace text as the query if no bold is found.
	textRegex := regexp.MustCompile(`(?m)(\S+.+?)\n\n`)

	for blocknum, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			Assert(false, "Empty block found at block number %d", blocknum)
			Fpf(os.Stderr, "WARN: Skipping empty block %d\n", blocknum)
			continue
		}
		Fpf(os.Stderr, "INFO: Processing block %d length %d\n", blocknum, len(block))

		var rt RoundTrip

		// Find the first occurrence of a bold segment for the query.
		qmatches := boldRegex.FindStringSubmatch(block)
		Fpf(os.Stderr, "INFO: block %d, qmatches length: %d\n", blocknum, len(qmatches))
		if len(qmatches) >= 3 {
			rt.Query = strings.TrimSpace(qmatches[1])
			Fpf(os.Stderr, "INFO: block %d, query: \n%s\n", blocknum, rt.Query)
		} else {
			// use the first text block as the query if no bold text is found
			qmatches = textRegex.FindStringSubmatch(block)
			if len(qmatches) >= 2 {
				rt.Query = strings.TrimSpace(qmatches[1])
				Fpf(os.Stderr, "INFO: block %d, no bold found, using first text block as query:\n%s\n", blocknum, rt.Query)
			} else {
				Assert(false, "No query found in block %d:\n\n%s", blocknum, block)
				Fpf(os.Stderr, "WARN: No query found in block %d, skipping block\n", blocknum)
				continue
			}
		}

		// Locate section markers "## References" and "## Reasoning"
		referencesMarker := "## References"
		reasoningMarker := "## Reasoning"

		idxRef := strings.Index(block, referencesMarker)
		idxThink := strings.Index(block, reasoningMarker)

		// Find Response
		// If a "## References" marker exists, we define the response as the part from the end of the query
		// up to the marker.
		if idxRef > 0 {
			// Find end of first bold query (if any) and then slice to
			// get the response.
			qEndIdx := 0
			if qmatches != nil {
				index := strings.Index(block, qmatches[0])
				if index != -1 {
					qEndIdx = index + len(qmatches[0])
				}
			}
			fmt.Fprintf(os.Stderr, "INFO: block %d, qEndIdx: %d, idxRef: %d, idxThink: %d, block length: %d\n",
				blocknum, qEndIdx, idxRef, idxThink, len(block))
			rt.Response = strings.TrimSpace(block[qEndIdx:idxRef])
		} else {
			fmt.Fprintf(os.Stderr, "WARN: No references marker found, block number %d\n", blocknum)
			// If no references marker exists, take all after the query as response.
			queryEndIdx := len(block)
			if qmatches != nil {
				index := strings.Index(block, qmatches[0])
				queryEndIdx = index + len(qmatches[0])
			}
			rt.Response = strings.TrimSpace(block[queryEndIdx:])
		}

		// Determine References and Reasoning if available.
		if idxRef != -1 && idxThink != -1 && idxThink > idxRef {
			rt.References = strings.TrimSpace(block[idxRef+len(referencesMarker) : idxThink])
			rt.Reasoning = strings.TrimSpace(block[idxThink+len(reasoningMarker):])
		} else if idxRef != -1 {
			// Assert(false, "No reasoning marker found in block %d", blocknum)
			// If only References marker exists, take all after as references.
			fmt.Fprintf(os.Stderr, "WARN: Only references marker found, block number %d\n", blocknum)
			rt.References = strings.TrimSpace(block[idxRef+len(referencesMarker):])
			rt.Reasoning = ""
		} else if idxThink != -1 {
			// Assert(false, "No references marker found in block %d", blocknum)
			// If only Reasoning marker exists, assign empty references.
			fmt.Fprintf(os.Stderr, "WARN: Only reasoning marker found, block number %d\n", blocknum)
			rt.References = ""
			rt.Reasoning = strings.TrimSpace(block[idxThink+len(reasoningMarker):])
		}

		// If no query was found (and thus possibly no valid roundtrip), we can decide whether to drop or include.
		if rt.Query == "" && rt.Response == "" && rt.References == "" && rt.Reasoning == "" {
			// Likely not a valid roundtrip block.
			Assert(false, "Empty or invalid roundtrip block at block %d\n", blocknum)
			continue
		}

		rounds = append(rounds, rt)
	}

	if len(rounds) == 0 && len(data) > 0 {
		return nil, errors.New("no valid roundtrips found in storm file")
	}

	fmt.Fprintf(os.Stderr, "INFO: Parsed %d roundtrips from storm file\n", len(rounds))

	return rounds, nil
}

// For debugging: a helper function to pretty-print a RoundTrip.
func (rt RoundTrip) String() string {
	var buf bytes.Buffer
	buf.WriteString("Query:\n" + rt.Query + "\n")
	buf.WriteString("Response:\n" + rt.Response + "\n")
	if rt.References != "" {
		buf.WriteString("References:\n" + rt.References + "\n")
	}
	if rt.Reasoning != "" {
		buf.WriteString("Reasoning:\n" + rt.Reasoning + "\n")
	}
	return buf.String()
}
