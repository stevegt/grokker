package core

import (
	"strings"

	. "github.com/stevegt/goadapt"
)

/*
var GitSummaryPrompt = `
Summarize the context into a single line of 60 characters or less.  Add nothing else.  Use present tense.  Do not quote.
`
var GitCommitPrompt = `
Summarize the bullet points found in the context into a single line of 60 characters or less.  Add nothing else.  Use present tense. Do not quote.
`
*/

var GitSummaryPrompt = `
In a single line of 60 characters or less, describe the changes
described in the context.
Use present tense, active, imperative statements as if giving directions.
Add nothing else.  Never add quote marks.
`

var GitDiffPrompt = `
In bullet points, describe the changes found in the 'git diff'
fragments in the context.  The bullet points will be used in the body
of a git commit message.
Use present tense, active, imperative statements as if giving directions.
Add nothing else.  Never add quote marks.
`

// summarizeDiff recursively summarizes a diff until the summary is
// short enough to be used as a prompt.
func (g *Grokker) summarizeDiff(diff string) (sumlines string, diffSummary string, err error) {
	defer Return(&err)
	maxTokens := int(float64(g.ModelObj.TokenLimit) * .7)
	// split the diff on filenames
	fileChunks := strings.Split(diff, "diff --git")
	// split each file chunk into smaller chunks
	for _, fileChunk := range fileChunks {
		// skip empty chunks
		if len(fileChunk) == 0 {
			continue
		}
		// get the filenames (they were right after the "diff --git"
		// string, on the same line)
		lines := strings.Split(fileChunk, "\n")
		var fns string
		if len(lines) > 0 {
			fns = lines[0]
		} else {
			fns = "a b"
		}
		var fileSummary string
		if len(fns) > 0 {
			fileSummary = Spf("summary of diff --git %s\n", fns)
		}
		var chunks []*Chunk
		chunks, err = g.chunksFromString(nil, fileChunk, maxTokens)
		// summarize each chunk
		for _, chunk := range chunks {
			// format the chunk
			context := Spf("diff --git %s\n%s", fns, chunk.text)
			resp, err := g.AnswerWithRAG(SysMsgChat, GitDiffPrompt, context, false)
			Ck(err)
			fileSummary = Spf("%s\n%s", fileSummary, resp)
		}
		// XXX recurse here to glue the summaries together for a given
		// file?

		// get a summary line of the changes for this file
		sumLine, err := g.AnswerWithRAG(SysMsgChat, GitSummaryPrompt, fileSummary, false)
		Ck(err)
		// append the summary line to the list of summary lines
		sumlines = Spf("%s\n%s", sumlines, sumLine)
		// append sumLine and the diff for this file to the summary
		// of the changes for all files
		diffSummary = Spf("%s\n\n%s\n\n%s", diffSummary, sumLine, fileSummary)
	}
	/*
		// if the summary is too long, recurse
		if len(diffSummary) > int(maxLen) {
			// recurse
			Fpf(os.Stderr, "diff summary too long (%d bytes), recursing\n", len(diffSummary))
			diffSummary, err = g.summarizeDiff(diffSummary)
		}
	*/
	return
}
