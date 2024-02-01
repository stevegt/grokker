package core

import (
	"path/filepath"

	. "github.com/stevegt/goadapt"
)

// Document is a single document in a document repository.
type Document struct {
	// XXX deprecated because we weren't precise about what it meant.
	Path string
	// The path to the document file, relative to g.Root
	RelPath string
}

// absPath returns the absolute path of a document.
func (g *GrokkerInternal) absPath(doc *Document) string {
	return filepath.Join(g.Root, doc.RelPath)
}

// updateDocument updates the embeddings for a document and returns
// true if the document was updated.
func (g *GrokkerInternal) updateDocument(doc *Document) (updated bool, err error) {
	defer Return(&err)
	// XXX much of this code is inefficient and will be replaced
	// when we have a kv store.
	Debug("updating embeddings for %s ...", doc.RelPath)

	// mark all existing chunks as stale
	for _, chunk := range g.Chunks {
		if chunk.Document.RelPath == doc.RelPath {
			chunk.stale = true
		}
	}

	// break the current doc up into chunks.
	chunks, err := g.chunksFromDoc(doc)
	Ck(err)
	// For each chunk, ensure it exists in the database with the right
	// hash, offset, and length.  We'll get embeddings later.
	var newChunks []*Chunk
	for _, chunk := range chunks {
		// setChunk unsets the stale bit if the chunk is already in the
		// database.
		// XXX move the stale bit unset to this loop instead, for readability.
		newChunk := g.setChunk(chunk)
		if newChunk != nil {
			updated = true
			newChunks = append(newChunks, newChunk)
		}
	}
	Debug("found %d new chunks", len(newChunks))
	// orphaned chunks will be garbage collected.

	// For each new chunk, generate an embedding using the
	// openai.Embedding.create() function. Store the embeddings for each
	// chunk in a data structure such as a list or dictionary.
	var newChunkStrings []string
	for _, chunk := range newChunks {
		Assert(chunk.Document.RelPath == doc.RelPath, "chunk document does not match")
		Assert(len(chunk.text) > 0, "chunk text is empty")
		Assert(chunk.Embedding == nil, "chunk embedding is not nil")
		Assert(chunk.stale == false, "chunk is stale")
		Assert(chunk.Hash != "", "chunk hash is empty")
		text, err := g.chunkText(chunk, true, false)
		Ck(err)
		newChunkStrings = append(newChunkStrings, text)
	}
	embeddings, err := g.createEmbeddings(newChunkStrings)
	Ck(err)
	for i, chunk := range newChunks {
		chunk.Embedding = embeddings[i]
	}
	return
}
