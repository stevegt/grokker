package core

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/semver"
)

// canon returns our best guess at the canonical path of a
// document, and warns if the document is not found at the
// canonical path.
func (g *GrokkerInternal) canon(path string) (rel string) {
	rel, err := filepath.Rel(g.Root, path)
	if err != nil {
		Fpf(os.Stderr, "error: %s\n", err)
	}
	// check if the document exists at the canonical path
	_, err = os.Stat(g.absPath(&Document{RelPath: rel}))
	if err != nil {
		Fpf(os.Stderr, "warning: document %q not found at canonical path %q\n", path, rel)
	}
	return
}

// migrate migrates the current Grokker database from an older version
// to the current version.
func (g *GrokkerInternal) migrate() (migrated bool, was, now string, err error) {
	defer Return(&err)

	was = g.Version
	now = g.Version

	// set default version
	if g.Version == "" {
		g.Version = "0.1.0"
	}

	// loop until migrations are done
	for {

		// check if migration is necessary
		var dbver, codever *semver.Version
		dbver, err = semver.Parse([]byte(g.Version))
		Ck(err)
		codever, err = semver.Parse([]byte(version))
		Ck(err)
		if semver.Cmp(dbver, codever) == 0 {
			// no migration necessary
			break
		}

		// see if db is newer version than code
		if semver.Cmp(dbver, codever) > 0 {
			// db is newer than code
			err = fmt.Errorf("grokker db is version %s, but you're running version %s -- upgrade grokker", g.Version, version)
			return
		}

		Fpf(os.Stderr, "migrating from %s to %s\n", g.Version, version)

		// if we get here, then dbver < codever
		_, minor, patch := semver.Upgrade(dbver, codever)
		Assert(patch, "patch should be true: %s -> %s", dbver, codever)

		// figure out what kind of migration we need to do
		if minor {
			// minor version changed; db migration necessary
			err = g.migrateOneVersion()
			Ck(err)
		} else {
			// only patch version changed; a patch version change is
			// just a code change, so just update the version number
			// in the db
			g.Version = version
		}

		migrated = true
	}

	now = g.Version

	return
}

// migrateOneVersion migrates the grok file from the current version to the
// next version.
func (g *GrokkerInternal) migrateOneVersion() (err error) {
	defer Return(&err)

	// we only care about the major and minor version numbers
	v, err := semver.Parse([]byte(g.Version))
	Ck(err)
	vstr := fmt.Sprintf("%s.%s.X", v.Major, v.Minor)

	switch vstr {

	case "0.1.X":
		// copy Document.Path to Document.RelPath, leave old content
		// for now
		for _, doc := range g.Documents {
			doc.RelPath = g.canon(doc.Path)
		}
		// copy Chunk.Document.Path to Chunk.Document.RelPath, leave
		// old content for now
		for _, chunk := range g.Chunks {
			// copy and canonicalize the path
			chunk.Document.RelPath = g.canon(chunk.Document.Path)
		}
		// refresh embeddings now because we are about to save the grok file
		// and that will make its timestamp newer than any possibly-modified
		// documents
		err = g.Setup(g.Model)
		Ck(err)
		err = g.RefreshEmbeddings()
		Ck(err)
		g.Version = "1.0.0"

	case "1.0.X":
		// add file paths to chunks -- all we need to do here is refresh
		// all of the doc chunks to get the file paths added
		err = g.Setup(g.Model)
		Ck(err)
		err = g.RefreshEmbeddings()
		Ck(err)
		g.Version = "1.1.0"

	case "1.1.X":
		// API change, so this is a no-op as far as the db is concerned
		g.Version = "2.0.0"

	case "2.0.X":
		// replace Text field with Hash, Offset, and Length fields in
		// Chunk struct -- all we need to do here is refresh all of the
		// doc chunks to get the new fields added
		err = g.Setup(g.Model)
		Ck(err)
		err = g.RefreshEmbeddings()
		Ck(err)
		g.Version = "2.1.0"

	case "2.1.X":
		// API change, so this is a no-op as far as the db is concerned
		g.Version = "3.0.0"

	// XXX remove doc.Path in a future version

	default:
		Assert(false, "migration missing: from version: %s", g.Version)
	}
	return
}
