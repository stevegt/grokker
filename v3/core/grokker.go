package core

import (
	"io/ioutil"
	"os"
	"time"

	"github.com/fabiustech/openai"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/util"
	"github.com/tiktoken-go/tokenizer"
)

// Grokker is a library for analyzing a set of documents and asking
// questions about them using the OpenAI chat and embeddings APIs.
//
// It uses this algorithm (generated by ChatGPT):
//
// To use embeddings in conjunction with the OpenAI Chat API to
// analyze a document, you can follow these general steps:
//
// (1) Break up the document into smaller text chunks or passages,
// each with a length of up to 8192 tokens (the maximum input size for
// the text-embedding-ada-002 model used by the Embeddings API).
//
// (2) For each text chunk, generate an embedding using the
// openai.Embedding.create() function. Store the embeddings for each
// chunk in a data structure such as a list or dictionary.
//
// (3) Use the Chat API to ask questions about the document. To do
// this, you can use the openai.Completion.create() function,
// providing the text of the previous conversation as the prompt
// parameter.
//
// (4) When a question is asked, use the embeddings of the document
// chunks to find the most relevant passages for the question. You can
// use a similarity measure such as cosine similarity to compare the
// embeddings of the question and each document chunk, and return the
// chunks with the highest similarity scores.
//
// (5) Provide the most relevant document chunks to the
// openai.Completion.create() function as additional context for
// generating a response. This will allow the model to better
// understand the context of the question and generate a more relevant
// response.
//
// Repeat steps 3-5 for each question asked, updating the conversation
// prompt as needed.

const (
	// See the "Semantic Versioning" section of the README for
	// information on API and db stability and versioning.
	Version = "3.0.36"
)

type Grokker struct {
	embeddingClient *openai.Client
	// The grokker version number this db was last updated with.
	Version string
	// The absolute path of the root directory of the document
	// repository.  This is passed in from cli based on where we
	// found the db.
	Root string
	// The list of documents in the database.
	Documents []*Document
	// The list of chunks in the database.
	Chunks []*Chunk
	// model specs
	models *Models
	// XXX make Model be the most recently used model name
	Model               string
	ModelObj            *Model `json:"-"`
	EmbeddingTokenLimit int
	// pathname of the grokker database file
	grokpath string
	// lock                *flock.Flock
}

// XXX get rid of this global
var Tokenizer tokenizer.Codec

// mtime returns the last modified time of the Grokker database.
func (g *Grokker) mtime() (timestamp time.Time, err error) {
	defer Return(&err)
	fi, err := os.Stat(g.grokpath)
	Ck(err)
	timestamp = fi.ModTime()
	return
}

// tokens returns the tokens for a text segment.
func (g *Grokker) tokens(text string) (tokens []string, err error) {
	defer Return(&err)
	_, tokens, err = Tokenizer.Encode(text)
	Ck(err)
	return
}

// meanVectorFromLongString returns the mean vector of a long string.
func (g *Grokker) meanVectorFromLongString(text string) (vector []float64, err error) {
	defer Return(&err)
	// break up the text into strings smaller than the token limit
	texts, err := g.stringsFromString(text, g.ModelObj.TokenLimit)
	Ck(err)
	// get the embeddings for each string
	embeddings, err := g.createEmbeddings(texts)
	Ck(err)
	// get the mean vector of the embeddings
	vector = util.MeanVector(embeddings)
	return
}

// TmpTestDir returns a temporary directory for testing
func TmpTestDir() string {
	dir, err := ioutil.TempDir("/tmp", "grokker-test")
	Ck(err)
	return dir
}
