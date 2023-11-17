package grokker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"

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

// NewChunk creates a new chunk given an offset, length, and text. It
// computes the sha256 hash of the text if doc is not nil.  It does
// not compute the embedding or add the chunk to the db.
func NewChunk(doc *Document, offset, length int, text string) (c *Chunk) {
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
	Debug("NewChunk: %#v", c)
	return
}

// TokenCount returns the number of tokens in a chunk, and caches the
// result in the chunk.
func (chunk *Chunk) TokenCount(g *Grokker) (count int, err error) {
	defer Return(&err)
	if chunk.tokenLength == 0 {
		text, err := g.ChunkText(chunk, false)
		Ck(err)
		tokens, err := g.Tokens(text)
		Ck(err)
		chunk.tokenLength = len(tokens)
	}
	count = chunk.tokenLength
	return
}

// splitChunk recursively splits a Chunk into smaller chunks until
// each chunk is no longer than the token limit.
func (chunk *Chunk) splitChunk(g *Grokker, tokenLimit int) (newChunks []*Chunk, err error) {
	defer Return(&err)
	// if the chunk is short enough, then we're done
	tc, err := chunk.TokenCount(g)
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
		text, err = g.ChunkText(chunk, false)
		Ck(err)
		subChunk := NewChunk(chunk.Document, docOffset, end-start, text[start:end])
		// recurse
		Debug("splitting subChunk %d of %d ...", i+1, numChunks)
		var newSubChunks []*Chunk
		newSubChunks, err = subChunk.splitChunk(g, tokenLimit)
		Ck(err)
		newChunks = append(newChunks, newSubChunks...)
	}
	return
}
