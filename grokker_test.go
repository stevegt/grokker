package grokker

import (
	"io/ioutil"
	"os"
	"testing"

	. "github.com/stevegt/goadapt"
)

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

func TestEmbeddings(t *testing.T) {
	//
	// create a new Grokker database
	grok, err := New(tempDir, "gpt-3.5-turbo")
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
	grok, err := New(tempDir, "gpt-3.5-turbo")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// add the document
	err = grok.AddDocument("testdata/te-abstract.txt")
	Tassert(t, err == nil, "error adding doc: %v", err)
}

// test finding chunks that are similar to a query
func TestFindSimilar(t *testing.T) {
	// create a new Grokker database
	grok, err := New(tempDir, "gpt-3.5-turbo")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// add the document
	err = grok.AddDocument("testdata/te-abstract.txt")
	Tassert(t, err == nil, "error adding doc: %v", err)
	// find similar chunks
	chunks, err := grok.FindChunks("Why is order of operations important when administering a UNIX machine?", 3)
	Tassert(t, err == nil, "error finding similar chunks: %v", err)
	Pl("similar chunks:")
	for _, chunk := range chunks {
		Pl(chunk.Text)
		Pl()
	}
}

// test a chat query
func TestChatQuery(t *testing.T) {
	// create a new Grokker database
	grok, err := New(tempDir, "gpt-3.5-turbo")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// add the document
	err = grok.AddDocument("testdata/te-abstract.txt")
	Tassert(t, err == nil, "error adding doc: %v", err)
	// answer the query
	resp, query, err := grok.Answer("Why is order of operations important when administering a UNIX machine?", false)
	Tassert(t, err == nil, "error answering query: %v", err)
	Pl("query:")
	Pl(query)
	Pl()
	Pl("answer:")
	Pprint(resp)
}
