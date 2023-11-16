package grokker

import (
	"io/ioutil"
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

func TestEmbeddings(t *testing.T) {
	//
	// create a new Grokker database
	grok, err := Init(tmpDir(), "gpt-3.5-turbo")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// add some embeddings
	embs, err := grok.CreateEmbeddings([]string{"hello", "world"})
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

// test finding chunks that are similar to a query
func TestFindSimilar(t *testing.T) {
	// create a new Grokker database
	grok, err := Init(tmpDir(), "gpt-3.5-turbo")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// add the document
	err = grok.AddDocument("testdata/te-abstract.txt")
	Tassert(t, err == nil, "error adding doc: %v", err)
	// find similar chunks
	chunks, err := grok.FindChunks("Why is order of operations important when administering a UNIX machine?", 2000)
	Tassert(t, err == nil, "error finding similar chunks: %v", err)
	Pl("similar chunks:")
	for _, chunk := range chunks {
		text, err := grok.ChunkText(chunk, true)
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
	chunks, err := grok.FindChunks("Why is order of operations important when administering a UNIX machine?", 2000)
	for _, chunk := range chunks {
		text, err := grok.ChunkText(chunk, true)
		Tassert(t, err == nil, "error getting chunk text: %v", err)
		Tassert(t, len(text) > 0, "expected chunk text to be non-empty")
		// tokenize the text
		tokens, err := grok.Tokens(text)
		Tassert(t, err == nil, "error tokenizing text: %v", err)
		Tassert(t, len(tokens) <= grok.tokenLimit, "expected %d tokens or less, got %d", grok.tokenLimit, len(tokens))
		Tassert(t, len(tokens) > 20, "expected more than 20 tokens, got %d", len(tokens))
	}
}

// test Revise()
func TestRevise(t *testing.T) {
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
}
