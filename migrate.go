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

func migrate_0_1_0_to_1_0_0(g *Grokker) {
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
	g.Version = "1.0.0"
}
