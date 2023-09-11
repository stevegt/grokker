package grokker

import (
	"os"
	"path/filepath"

	. "github.com/stevegt/goadapt"
)

// canon returns our best guess at the canonical path of a
// document, and warns if the document is not found at the
// canonical path.
func canon(g *Grokker, path string) (rel string) {
	rel, err := filepath.Rel(g.Root, path)
	if err != nil {
		Fpf(os.Stderr, "error: %s\n", err)
	}
	// check if the document exists at the canonical path
	_, err = os.Stat(g.AbsPath(&Document{RelPath: rel}))
	if err != nil {
		Fpf(os.Stderr, "warning: document %q not found at canonical path %q\n", path, rel)
	}
	return
}

func migrate_0_1_0_to_1_0_0(g *Grokker) (err error) {
	defer Return(&err)
	// copy Document.Path to Document.RelPath, leave old content
	// for now
	for _, doc := range g.Documents {
		doc.RelPath = canon(g, doc.Path)
	}
	// copy Chunk.Document.Path to Chunk.Document.RelPath, leave
	// old content for now
	for _, chunk := range g.Chunks {
		// copy and canonicalize the path
		chunk.Document.RelPath = canon(g, chunk.Document.Path)
	}
	// refresh embeddings now because we are about to save the grok file
	// and that will make its timestamp newer than any possibly-modified
	// documents
	err = g.setup(g.Model)
	Ck(err)
	err = g.RefreshEmbeddings()
	Ck(err)
	g.Version = "1.0.0"
	return
}

func migrate_1_0_0_to_1_1_0(g *Grokker) (err error) {
	defer Return(&err)
	// add file paths to chunks -- all we need to do here is refresh
	// all of the doc chunks to get the file paths added
	err = g.setup(g.Model)
	Ck(err)
	err = g.RefreshEmbeddings()
	Ck(err)
	g.Version = "1.1.0"
	return
}

func migrate_1_1_0_to_2_0_0(g *Grokker) (err error) {
	defer Return(&err)
	// API change, so this is a no-op as far as the db is concerned
	g.Version = "2.0.0"
	return
}
