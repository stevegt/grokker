package grokker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"sort"
	"strings"

	. "github.com/stevegt/goadapt"
)

// Chunk is a single chunk of text from a document.
type Chunk struct {
	// The document that this chunk is from.
	Document *Document
	// The offset of the chunk in the document.
	Offset int
	// The length of the chunk in the document.
	Length int
	// XXX store tokenLength in database and stop recomputing it
	tokenLength int
	// sha256 hash of the text of the chunk.
	Hash string
	// The text of the chunk.  This is not stored in the db.
	text string
	// The embedding of the chunk.
	Embedding []float64
	// The grokker that this chunk belongs to.
	// g *Grokker
	// true if needs to be garbage collected
	stale bool
}

// newChunk creates a new chunk given an offset, length, and text. It
// computes the sha256 hash of the text if doc is not nil.  It does
// not compute the embedding or add the chunk to the db.
func newChunk(doc *Document, offset, length int, text string) (c *Chunk) {
	var prefixedText string
	var hashStr string
	if doc != nil {
		prefixedText = fmt.Sprintf("from %s:\n%s\n", doc.RelPath, text)
		hash := sha256.Sum256([]byte(prefixedText))
		hashStr = hex.EncodeToString(hash[:])
	}
	c = &Chunk{
		// g:        g,
		Document: doc,
		Offset:   offset,
		Length:   length,
		Hash:     hashStr,
		text:     text,
	}
	// Debug("NewChunk: %#v", c)
	return
}

// splitChunk recursively splits a Chunk into smaller chunks until
// each chunk is no longer than the token limit.
func (chunk *Chunk) splitChunk(g *GrokkerInternal, tokenLimit int) (newChunks []*Chunk, err error) {
	defer Return(&err)
	// if the chunk is short enough, then we're done
	tc, err := chunk.tokenCount(g)
	Ck(err)
	Debug("chunk token count: %d, token limit: %d", tc, tokenLimit)
	if tc < tokenLimit {
		newChunks = append(newChunks, chunk)
		Debug("chunk is short enough")
		return
	}
	// split chunk into smaller segments of roughly equal size
	// XXX could be made smarter by splitting on sentence or context boundaries
	numChunks := int(math.Ceil(float64(tc) / float64(tokenLimit)))
	chunkSize := int(math.Ceil(float64(chunk.Length) / float64(numChunks)))
	Debug("splitting chunk into %d chunks of size %d ...", numChunks, chunkSize)
	for i := 0; i < numChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > chunk.Length {
			end = chunk.Length
		}
		docOffset := chunk.Offset + start
		var text string
		text, err = g.chunkText(chunk, false, false)
		Ck(err)
		subChunk := newChunk(chunk.Document, docOffset, end-start, text[start:end])
		// recurse
		Debug("splitting subChunk %d of %d ...", i+1, numChunks)
		var newSubChunks []*Chunk
		newSubChunks, err = subChunk.splitChunk(g, tokenLimit)
		Ck(err)
		newChunks = append(newChunks, newSubChunks...)
	}
	return
}

// chunkText returns the text of a chunk.
func (g *GrokkerInternal) chunkText(c *Chunk, withHeader, withLineNumbers bool) (text string, err error) {
	// Debug("ChunkText(%#v)", c)
	if c.Document == nil {
		Assert(c.text != "", "ChunkText: c.Document == nil && c.text == \"\"")
		text = c.text
		return
	}

	// read the chunk from the document
	var buf []byte
	buf, err = ioutil.ReadFile(g.absPath(c.Document))
	if os.IsNotExist(err) {
		// document has been removed; don't remove it from the
		// database, but don't return any text either.  The
		// document might be on a different branch in e.g. git.
		err = nil
		Debug("ChunkText: document %q not found", c.Document.RelPath)
		return
	}
	Ck(err)
	start := c.Offset
	stop := c.Offset + c.Length
	if start >= len(buf) {
		start = len(buf) - 1
	}
	if stop > len(buf) {
		stop = len(buf)
	}
	rawText := string(buf[start:stop])
	if withLineNumbers {
		// count the lines before start
		// XXX this is inefficient because it has to be done for
		// every chunk.  it would be better to do it once for
		// the whole document and store that in the db or at least
		// cache it during a single grok run.
		docLines := strings.Split(string(buf[:start]), "\n")
		startLine := len(docLines)
		// add line numbers to the text
		chunkLines := strings.Split(rawText, "\n")
		for i := startLine; i < startLine+len(chunkLines); i++ {
			// get the text of the line
			lineTxt := chunkLines[i-startLine]
			// add the line number
			text += fmt.Sprintf("%d: %s\n", i, lineTxt)
		}
	} else {
		text = rawText
	}
	if withHeader {
		text = fmt.Sprintf("from %s:\n%s\n", c.Document.RelPath, text)
	}

	// Debug("ChunkText: %q", text)
	return
}

// splitIntoChunks splits a string into a slice of chunks using the
// given delimiter, returning each string as a partially populated
// Chunk with the offset set to the start of the string.
func splitIntoChunks(doc *Document, txt, delimiter string) (chunks []*Chunk) {
	start := 0
	i := 0
	for done := false; !done; i++ {
		var t string
		if i >= len(txt)-len(delimiter) {
			// we're at the end of the document: put the
			// remaining text into a chunk.
			t = txt[start:]
			done = true
		} else if txt[i:i+len(delimiter)] == delimiter {
			// found a delimiter
			t = txt[start : i+len(delimiter)]
		}
		// if we found a delimiter, then create a chunk
		// and add it to the list of chunks.
		if t != "" {
			chunk := newChunk(doc, start, len(t), t)
			chunks = append(chunks, chunk)
			// start the next chunk after the delimiter
			start = i + len(delimiter)
			i = start
			continue
		}
	}
	return
}

// similarChunks returns the most similar chunks to an embedding,
// limited by tokenLimit.
func (g *GrokkerInternal) similarChunks(embedding []float64, tokenLimit int, files []string) (chunks []*Chunk, err error) {
	defer Return(&err)
	Debug("chunks in database: %d", len(g.Chunks))
	// Assert(tokenLimit > 100, tokenLimit)
	// find the most similar chunks.
	type Sim struct {
		chunk *Chunk
		score float64
	}
	sims := make([]Sim, 0, len(g.Chunks))
	for _, chunk := range g.Chunks {
		// skip chunks from other files if files is not nil
		if files != nil {
			var found bool
			for _, file := range files {
				if chunk.Document.RelPath == file {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		score := similarity(embedding, chunk.Embedding)
		sims = append(sims, Sim{chunk, score})
	}
	// sort the chunks by similarity.
	sort.Slice(sims, func(i, j int) bool {
		return sims[i].score > sims[j].score
	})
	// collect the top chunks until we pass the token limit
	var totalTokens int
	var bigChunks []*Chunk
	for _, sim := range sims {
		chunk := sim.chunk
		tc, err := chunk.tokenCount(g)
		Ck(err)
		totalTokens += tc
		bigChunks = append(bigChunks, chunk)
		if totalTokens > tokenLimit {
			break
		}
	}
	// split the big chunks so none are larger than the token limit.
	// stop before we reach the token limit.
	totalTokens = 0
	for _, chunk := range bigChunks {
		var subChunks []*Chunk
		subChunks, err = chunk.splitChunk(g, tokenLimit)
		Ck(err)
		for _, subChunk := range subChunks {
			tc, err := subChunk.tokenCount(g)
			Ck(err)
			totalTokens += tc
			if totalTokens > tokenLimit {
				break
			}
			chunks = append(chunks, subChunk)
		}
		if totalTokens > tokenLimit {
			break
		}
	}
	Debug("sims len: %d", len(sims))
	Debug("total tokens: %d", totalTokens)
	Debug("found %d similar chunks", len(chunks))
	return
}

// findChunks returns the most relevant chunks for a query, limited by tokenLimit.
func (g *GrokkerInternal) findChunks(query string, tokenLimit int, files []string) (chunks []*Chunk, err error) {
	defer Return(&err)
	// get the embeddings for the query.
	embeddings, err := g.createEmbeddings([]string{query})
	Ck(err)
	queryEmbedding := embeddings[0]
	if queryEmbedding == nil {
		return
	}
	// find the most similar chunks.
	chunks, err = g.similarChunks(queryEmbedding, tokenLimit, files)
	Ck(err)
	return
}

// stringsFromString splits a string into a slice of strings.  Each
// string will be no longer than tokenLimit tokens.
func (g *GrokkerInternal) stringsFromString(txt string, tokenLimit int) (strings []string, err error) {
	defer Return(&err)
	Assert(tokenLimit > 0)
	chunks, err := g.chunksFromString(nil, txt, tokenLimit)
	Ck(err)
	for _, chunk := range chunks {
		strings = append(strings, chunk.text)
	}
	return
}

// chunksFromString splits a string into a slice of Chunks.  If doc is
// not nil, it is used to set the Document field of each chunk.  Each
// chunk will be no longer than tokenLimit tokens.
func (g *GrokkerInternal) chunksFromString(doc *Document, txt string, tokenLimit int) (chunks []*Chunk, err error) {
	defer Return(&err)
	Assert(tokenLimit > 0)

	/*
		lang, _, _ := Ext2Lang(doc.RelPath)
		switch lang {
		case "go":
			chunks = lang.SplitGo(doc, txt)
		default:
			// split the text into paragraphs
			// XXX splitting on paragraphs is not ideal.  smarter splitting
			// might look at the structure of the text and split on
			// sections, chapters, etc.  it might also be useful to include
			// metadata such as file names.
			chunks = splitIntoChunks(doc, txt, "\n\n")
		}
	*/
	chunks = splitIntoChunks(doc, txt, "\n\n")

	// ensure no chunk is longer than the token limit
	var newChunks []*Chunk
	for _, chunk := range chunks {
		var subChunks []*Chunk
		subChunks, err = chunk.splitChunk(g, tokenLimit)
		Ck(err)
		newChunks = append(newChunks, subChunks...)
	}
	chunks = newChunks

	return
}

// chunksFromDoc returns a slice containing the chunks for a document.
func (g *GrokkerInternal) chunksFromDoc(doc *Document) (chunks []*Chunk, err error) {
	defer Return(&err)
	// read the document.
	buf, err := ioutil.ReadFile(g.absPath(doc))
	Ck(err)
	// break the document up into chunks.
	chunks, err = g.chunksFromString(doc, string(buf), g.embeddingTokenLimit)
	Ck(err)
	// add the document to each chunk.
	for _, chunk := range chunks {
		chunk.Document = doc
	}
	return
}

// setChunk ensures that a chunk exists in the database with the right
// doc, hash, offset, and length, and unsets the stale bit.  It
// returns the chunk if it was added to the database, or nil if it was
// already in the database. The caller needs to set the embedding if
// newChunk is not nil.
func (g *GrokkerInternal) setChunk(chunk *Chunk) (newChunk *Chunk) {
	// check if the chunk is already in the database.
	var foundChunk *Chunk
	for _, c := range g.Chunks {
		if c.Hash == chunk.Hash && c.Document.RelPath == chunk.Document.RelPath {
			foundChunk = c
			foundChunk.Offset = chunk.Offset
			foundChunk.Length = chunk.Length
			foundChunk.stale = false
		}
	}
	if foundChunk == nil {
		// add the chunk to the database.
		g.Chunks = append(g.Chunks, chunk)
		newChunk = chunk
		newChunk.stale = false
	}
	return
}

// gc removes any chunks that are marked as stale or that are orphaned.
func (g *GrokkerInternal) gc() (err error) {
	defer Return(&err)
	// build doc name map
	docMap := make(map[string]bool)
	for _, doc := range g.Documents {
		docMap[doc.RelPath] = true
	}
	oldLen := len(g.Chunks)
	var keepChunks []*Chunk
	for _, chunk := range g.Chunks {
		// check if chunk is referenced by any document.
		_, ok := docMap[chunk.Document.RelPath]
		if !ok {
			chunk.stale = true
		}
		// keep the chunk if it's not stale.
		if !chunk.stale {
			keepChunks = append(keepChunks, chunk)
		}
	}
	// replace the old chunks with the new chunks.
	g.Chunks = keepChunks
	newLen := len(g.Chunks)
	Debug("garbage collected %d chunks from the database", oldLen-newLen)
	return
}

// getContext returns the context for a query.
func (g *GrokkerInternal) getContext(query string, tokenLimit int, withHeaders, withLineNumbers bool, files []string) (context string, err error) {
	defer Return(&err)
	// Debug("getting context, tokenLimit: %d, query: %q", tokenLimit, query)
	// get chunks, sorted by similarity to the query.
	chunks, err := g.findChunks(query, tokenLimit, files)
	Ck(err)
	for _, chunk := range chunks {
		text, err := g.chunkText(chunk, withHeaders, withLineNumbers)
		Ck(err)
		context += text
	}
	Debug("using %d chunks as context", len(chunks))
	return
}

// tokenCount returns the number of tokens in a chunk, and caches the
// result in the chunk.
func (chunk *Chunk) tokenCount(g *GrokkerInternal) (count int, err error) {
	defer Return(&err)
	if chunk.tokenLength == 0 {
		text, err := g.chunkText(chunk, false, false)
		Ck(err)
		tokens, err := g.tokens(text)
		Ck(err)
		chunk.tokenLength = len(tokens)
	}
	count = chunk.tokenLength
	return
}
