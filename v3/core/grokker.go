3"
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
