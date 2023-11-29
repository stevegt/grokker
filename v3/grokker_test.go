package grokker

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	. "github.com/stevegt/goadapt"
)

/*
var tempDir string

// prior to starting test cases, create a temporary directory
func TestMain(m *testing.M) {
	// create a temporary directory
	var err error
	tempDir, err = ioutil.TempDir("/tmp", "grokker-test")
	Ck(err)
	// run the test cases
	os.Exit(m.Run())
}
*/

// tmpDir returns a temporary directory for testing
func tmpDir() string {
	dir, err := ioutil.TempDir("/tmp", "grokker-test")
	Ck(err)
	return dir
}

func TestSplitChunk(t *testing.T) {
	// create a new Grokker database
	grok, err := Init(tmpDir(), "gpt-3.5-turbo")
	Tassert(t, err == nil, "error creating grokker: %v", err)

	// create a new Chunk
	buf, err := ioutil.ReadFile("testdata/te-full.txt")
	Tassert(t, err == nil, "error reading testdata/te-full.txt: %v", err)
	text := string(buf)
	length := len(text)
	Tassert(t, length > 0, "expected text to be non-empty")
	Tassert(t, length > grok.tokenLimit, "expected text to be longer than %d tokens", grok.tokenLimit)
	chunk := newChunk(nil, 0, length, text)
	tc, err := chunk.tokenCount(grok)
	Tassert(t, err == nil, "error getting token count for chunk: %v", err)
	Tassert(t, tc > grok.tokenLimit, "expected chunk to have more than %d tokens, got %d tokens", grok.tokenLimit, tc)

	// call the splitChunk method
	newChunks, err := chunk.splitChunk(grok, 2000)
	Tassert(t, err == nil, "error splitting chunk: %v", err)

	// verify that the original chunk was split
	Tassert(t, len(newChunks) > 0, "expected chunk to be split into more than zero")
	if len(newChunks) == 1 {
		c := newChunks[0]
		tc, err := c.tokenCount(grok)
		Tassert(t, err == nil, "error getting token count for chunk: %v", err)
		Tassert(t, false, "expected chunk to be split, got %d tokens", tc)
	}

	// verify that each new chunk is within the token limit
	for i, ch := range newChunks {
		tc, err := ch.tokenCount(grok)
		Tassert(t, err == nil, "error getting token count for chunk %d: %v", i, err)
		Tassert(t, tc <= grok.tokenLimit, "expected chunk %d to have %d tokens or less, got %d tokens", i, grok.tokenLimit, tc)
	}
}
func TestEmbeddings(t *testing.T) {
	//
	// create a new Grokker database
	grok, err := Init(tmpDir(), "gpt-3.5-turbo")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// add some embeddings
	embs, err := grok.createEmbeddings([]string{"hello", "world"})
	Tassert(t, err == nil, "error creating embeddings: %v", err)
	Tassert(t, len(embs) == 2, "expected 2 embeddings, got %d", len(embs))
	Tassert(t, len(embs[0]) == 1536, "expected 1536 embeddings, got %d", len(embs[0]))
	// Pl("embeddings:")
	// Pprint(embs)
}

// test adding a document
func TestAddDoc(t *testing.T) {
	// create a new Grokker database
	grok, err := Init(tmpDir(), "gpt-3.5-turbo")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// add the document
	err = grok.AddDocument("testdata/te-abstract.txt")
	Tassert(t, err == nil, "error adding doc: %v", err)
}

func TestChunkTextAfterRemovingFile(t *testing.T) {
	// create a new Grokker database
	grok, err := Init(tmpDir(), "gpt-3.5-turbo")
	Tassert(t, err == nil, "error creating grokker: %v", err)

	// copy a document to a temporary file
	srcPath := "testdata/te-full.txt"
	docPath := "testdata/te-full-copy.txt"
	// remove the temporary file first, in case it already exists
	_ = os.Remove(docPath)
	err = copyFile(srcPath, docPath)
	Tassert(t, err == nil, "error copying file: %v", err)

	// add the temporary file to the database
	err = grok.AddDocument(docPath)
	Tassert(t, err == nil, "error adding doc: %v", err)

	// get chunks from the document
	chunks, err := grok.findChunks("Why is order of operations important when administering a UNIX machine?", 2000)
	Tassert(t, err == nil, "error finding chunks: %v", err)
	Tassert(t, len(chunks) > 0, "expected at least one chunk")
	chunk := chunks[0]
	Tassert(t, chunk != nil, "expected chunk to be non-nil")
	Tassert(t, len(chunk.text) > 0, "expected chunk text to be non-empty")

	// then remove the document from the file system
	err = os.Remove(docPath)
	Tassert(t, err == nil, "error removing document file: %v", err)

	// now get the text of the chunk from the removed document
	text, err := grok.chunkText(chunk, false)
	// check that there is no error and the text is empty
	Tassert(t, err == nil, "error getting chunk text: %v", err)
	Tassert(t, text == "", "expected chunk text to be empty")
}

// test finding chunks that are similar to a query
func TestFindSimilar(t *testing.T) {
	// create a new Grokker database
	grok, err := Init(tmpDir(), "gpt-3.5-turbo")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// add the document
	err = grok.AddDocument("testdata/te-abstract.txt")
	Tassert(t, err == nil, "error adding doc: %v", err)
	// find similar chunks
	chunks, err := grok.findChunks("Why is order of operations important when administering a UNIX machine?", 2000)
	Tassert(t, err == nil, "error finding similar chunks: %v", err)
	Pl("similar chunks:")
	for _, chunk := range chunks {
		text, err := grok.chunkText(chunk, true)
		Tassert(t, err == nil, "error getting chunk text: %v", err)
		Tassert(t, len(text) > 0, "expected chunk text to be non-empty")
	}
}

// test a chat query
func TestChatQuery(t *testing.T) {
	// create a new Grokker database
	grok, err := Init(tmpDir(), "gpt-3.5-turbo")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// add the document
	err = grok.AddDocument("testdata/te-abstract.txt")
	Tassert(t, err == nil, "error adding doc: %v", err)
	// answer the query
	query := "What is the cheapest and easiest way to make a set of changes to a set of machines if you want them all to behave the same when you're done?"
	Pl("query:", query)
	resp, err := grok.Answer(query, false)
	Tassert(t, err == nil, "error answering query: %v", err)
	Pl("answer:")
	Pprint(resp)

	// grok msg "You are an expert in the following topic.  Tell me if the provided text agrees with deterministic ordering.  Say 'answer=yes' or 'answer=no'."

}

// test splitting chunks when chunk size is greater than token limit
func TestSplitChunks(t *testing.T) {
	// create a new Grokker database
	grok, err := Init(tmpDir(), "gpt-3.5-turbo")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// add the document
	err = grok.AddDocument("testdata/te-full.txt")
	Tassert(t, err == nil, "error adding doc: %v", err)
	// get the chunks
	chunks, err := grok.findChunks("Why is order of operations important when administering a UNIX machine?", 2000)
	for _, chunk := range chunks {
		text, err := grok.chunkText(chunk, true)
		Tassert(t, err == nil, "error getting chunk text: %v", err)
		Tassert(t, len(text) > 0, "expected chunk text to be non-empty")
		// tokenize the text
		tokens, err := grok.tokens(text)
		Tassert(t, err == nil, "error tokenizing text: %v", err)
		Tassert(t, len(tokens) <= grok.tokenLimit, "expected %d tokens or less, got %d", grok.tokenLimit, len(tokens))
		Tassert(t, len(tokens) > 20, "expected more than 20 tokens, got %d", len(tokens))
	}
}

// test Revise()
func XXXTestRevise(t *testing.T) {
	// create a new Grokker database
	grok, err := Init(tmpDir(), "gpt-4")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// add documents
	err = grok.AddDocument("testdata/te-full.txt")
	Tassert(t, err == nil, "error adding doc: %v", err)
	// get text to revise from testdata/revise.txt
	text, err := ioutil.ReadFile("testdata/revise.txt")
	Tassert(t, err == nil, "error reading testdata/revise.txt: %v", err)
	// revise the text
	rev, _, err := grok.Revise(string(text), false, true)
	Tassert(t, err == nil, "error revising text: %v", err)
	// break the original text into paragraphs
	chunks := splitIntoChunks(nil, string(text), "\n\n")
	var paras []string
	for _, chunk := range chunks {
		paras = append(paras, chunk.text)
	}
	// break the revised text into paragraphs
	revchunks := splitIntoChunks(nil, string(rev), "\n\n")
	var revparas []string
	for _, chunk := range revchunks {
		revparas = append(revparas, chunk.text)
	}
	// ensure sysmsg hasn't changed
	sysmsg := strings.TrimSpace(paras[0])
	revsysmsg := strings.TrimSpace(revparas[0])
	Tassert(t, sysmsg == revsysmsg, "expected sysmsg to be unchanged: want:\n%s\ngot:\n%s", sysmsg, revsysmsg)
	// ensure there are at least two paragraphs in the revised text
	Tassert(t, len(revparas) >= 2, "expected at least two paragraphs in revised text, got %d", len(revparas))
	Pl("revised text:")
	Pl(rev)

	// grok msg "You are an expert in the following topic.  Say 'rating=N', where N is an integer from 0 to 100, where 0 means the provided text disregards the halting problem in systems administration, and 100 considers it paramount."  < testdata/revise.txt
}
