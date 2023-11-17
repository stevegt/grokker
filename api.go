package grokker

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/semver"
)

// AddDocument adds a document to the Grokker database. It creates the
// embeddings for the document and adds them to the database.
func (g *Grokker) AddDocument(path string) (err error) {
	defer Return(&err)
	// assume we're in an arbitrary directory, so we need to
	// convert the path to an absolute path.
	absPath, err := filepath.Abs(path)
	Ck(err)
	// always convert path to a relative path for consistency
	relpath, err := filepath.Rel(g.Root, absPath)
	doc := &Document{
		RelPath: relpath,
	}
	// ensure the document exists
	_, err = os.Stat(g.absPath(doc))
	if os.IsNotExist(err) {
		err = fmt.Errorf("not found: %s", doc.RelPath)
		return
	}
	Ck(err)
	// find out if the document is already in the database.
	found := false
	for _, d := range g.Documents {
		if d.RelPath == doc.RelPath {
			found = true
			break
		}
	}
	if !found {
		// add the document to the database.
		g.Documents = append(g.Documents, doc)
	}
	// update the embeddings for the document.
	_, err = g.updateDocument(doc)
	Ck(err)
	return
}

// ForgetDocument removes a document from the Grokker database.
func (g *Grokker) ForgetDocument(path string) (err error) {
	defer Return(&err)
	// remove the document from the database.
	for i, d := range g.Documents {
		match := false
		// try comparing the paths directly first.
		if d.RelPath == path {
			match = true
		}
		// if that doesn't work, try comparing the absolute paths.
		relpath, err := filepath.Abs(path)
		Ck(err)
		if g.absPath(d) == relpath {
			match = true
		}
		if match {
			Debug("forgetting document %s ...", path)
			g.Documents = append(g.Documents[:i], g.Documents[i+1:]...)
			break
		}
	}
	// the document chunks are still in the database, but they will be
	// removed during garbage collection.
	return
}

// Continue returns a continuation of the input text.
func (g *Grokker) Continue(in string, global bool) (out, sysmsg string, err error) {
	defer Return(&err)
	sysmsg = SysMsgContinue
	// tokenize sysmsg
	_, sysmsgTokens, err := g.tokenizer.Encode(sysmsg)
	Ck(err)
	// tokenize input
	_, inTokens, err := g.tokenizer.Encode(in)
	Ck(err)
	// get chunks, sorted by similarity to the txt.
	tokenLimit := int(float64(g.tokenLimit)*0.4) - len(sysmsgTokens) - len(inTokens)
	context, err := g.getContext(in, tokenLimit)
	Ck(err)
	// generate the answer.
	resp, err := g.generate(sysmsg, in, context, global)
	Ck(err)
	out = resp.Choices[0].Message.Content
	return
}

// Answer returns the answer to a question.
func (g *Grokker) Answer(question string, global bool) (resp string, err error) {
	defer Return(&err)
	// tokenize the question
	qtokens, err := g.tokens(question)
	Ck(err)
	maxTokens := int(float64(g.tokenLimit)*0.5) - len(qtokens)
	context, err := g.getContext(question, maxTokens)
	Ck(err)
	// generate the answer.
	respmsg, err := g.generate(SysMsgChat, question, context, global)
	resp = respmsg.Choices[0].Message.Content
	return
}

// Revise returns revised text based on input text.
func (g *Grokker) Revise(in string, global, sysmsgin bool) (out, sysmsg string, err error) {
	defer Return(&err)

	// tokenize the entire input
	inTokens, err := g.tokens(in)

	var body string
	if sysmsgin {
		// split input into sysmsg and txt
		paragraphs := strings.Split(in, "\n\n")
		if len(paragraphs) < 2 {
			err = fmt.Errorf("input must contain at least two paragraphs")
			return
		}
		sysmsg = paragraphs[0]
		// in = strings.Join(paragraphs[1:], "\n\n")
		body = strings.Join(paragraphs, "\n\n")
	} else {
		sysmsg = SysMsgRevise
	}

	// get context
	maxTokens := int(float64(g.tokenLimit)*0.5) - len(inTokens)
	context, err := g.getContext(body, maxTokens)
	Ck(err)

	// generate the answer.
	resp, err := g.generate(sysmsg, in, context, global)
	Ck(err)
	if sysmsgin {
		out = Spf("%s\n\n%s", sysmsg, resp.Choices[0].Message.Content)
	} else {
		out = resp.Choices[0].Message.Content
	}
	return
}

// Backup backs up the Grokker database to a time-stamped backup and
// returns the path.
func (g *Grokker) Backup() (backpath string, err error) {
	defer Return(&err)
	Assert(g.grokpath != "", "g.grokpath is empty")
	tmpdir := os.TempDir()
	deslashed := strings.Replace(g.grokpath, "/", "-", -1)
	backpath = fmt.Sprintf("%s/grokker-backup-%s%s", tmpdir, time.Now().Format("20060102-150405"), deslashed)
	err = copyFile(g.grokpath, backpath)
	Ck(err, "failed to backup %q to %q", g.grokpath, backpath)
	return
}

// Save saves the Grokker database as json data in an io.Writer.
func (g *Grokker) Save() (err error) {
	defer Return(&err)

	// open
	Debug("saving grok file")
	tmpfn := g.grokpath + ".tmp"
	fh, err := os.Create(tmpfn)

	// write
	data, err := json.Marshal(g)
	Ck(err)
	_, err = fh.Write(data)

	// close
	err = fh.Close()
	Ck(err)

	// move
	err = os.Rename(tmpfn, g.grokpath)
	Ck(err)
	Debug(" done!")

	return
}

// UpdateEmbeddings updates the embeddings for any documents that have
// changed since the last time the embeddings were updated.  It returns
// true if any embeddings were updated.
func (g *Grokker) UpdateEmbeddings() (update bool, err error) {
	defer Return(&err)
	// we use the timestamp of the grokfn as the last embedding update time.
	lastUpdate, err := g.mtime()
	Ck(err)
	for _, doc := range g.Documents {
		// check if the document has changed.
		fi, err := os.Stat(g.absPath(doc))
		if os.IsNotExist(err) {
			// document has been removed; don't remove it from the
			// database, but don't update it either.  We don't want
			// to remove it from the database because it might be
			// on a different branch in e.g. git.
			// g.ForgetDocument(g.AbsPath(doc))
			// update = true
			err = nil
			continue
		}
		Ck(err)
		if fi.ModTime().After(lastUpdate) {
			// update the embeddings.
			Debug("updating embeddings for %s ...", doc.RelPath)
			updated, err := g.updateDocument(doc)
			Ck(err)
			Debug("done\n")
			update = update || updated
		}
	}
	// garbage collect any chunks that are no longer referenced.
	g.gc()
	return
}

// CodeVersion returns the version of the grokker code.
func CodeVersion() string {
	return version
}

// DBVersion returns the version of the grokker database.
func (g *Grokker) DBVersion() string {
	return g.Version
}

// TokenCount returns the number of tokens in a string.
func (g *Grokker) TokenCount(text string) (count int, err error) {
	defer Return(&err)
	tokens, err := g.tokens(text)
	Ck(err)
	count = len(tokens)
	return
}

// RefreshEmbeddings refreshes the embeddings for all documents in the
// database.
func (g *Grokker) RefreshEmbeddings() (err error) {
	defer Return(&err)
	// regenerate the embeddings for each document.
	for _, doc := range g.Documents {
		Fpf(os.Stderr, "refreshing embeddings for %s\n", doc.RelPath)
		// remove file from list if it doesn't exist.
		absPath := g.absPath(doc)
		Debug("absPath: %s", absPath)
		_, err := os.Stat(absPath)
		Debug("stat err: %v", err)
		if os.IsNotExist(err) {
			// remove the document from the database.
			g.ForgetDocument(doc.RelPath)
			continue
		}
		_, err = g.updateDocument(doc)
		Ck(err)
	}
	g.gc()
	return
}

// ListDocuments returns a list of all documents in the knowledge base.
// XXX this is a bit of a hack, since we're using the document name as
// the document ID.
// XXX this is also a bit of a hack since we're trying to make this
// work for multiple versions -- we should be able to simplify this
// after migration is automatic during Load().
func (g *Grokker) ListDocuments() (paths []string) {
	for _, doc := range g.Documents {
		path := doc.Path
		v100, err := semver.Parse([]byte("1.0.0"))
		current, err := semver.Parse([]byte(g.Version))
		Ck(err)
		if semver.Cmp(current, v100) > 0 {
			path = doc.RelPath
		}
		paths = append(paths, path)
	}
	return
}

// LoadFrom loads a Grokker database from a given path.
// XXX replace the json db with a kv store, store vectors as binary
// floating point values.
func LoadFrom(grokpath string) (g *Grokker, migrated bool, oldver, newver string, err error) {
	g = &Grokker{}
	g.grokpath = grokpath
	// load the db
	fh, err := os.Open(g.grokpath)
	Ck(err)
	buf, err := ioutil.ReadAll(fh)
	Ck(err)
	err = json.Unmarshal(buf, g)
	Ck(err)
	// set the root directory, overriding whatever was in the db
	// - this is necessary because the db might have been moved
	g.Root, err = filepath.Abs(filepath.Dir(g.grokpath))
	Ck(err)

	migrated, oldver, newver, err = g.migrate()
	Ck(err)

	err = g.setup(g.Model)
	Ck(err)
	return
}

// Init creates a Grokker database in the given root directory.
func Init(rootdir, model string) (g *Grokker, err error) {
	defer Return(&err)
	g, err = InitNamed(rootdir, ".grok", model)
	return
}

// InitNamed creates a named Grokker database in the given root directory.
func InitNamed(rootdir, name, model string) (g *Grokker, err error) {
	// ensure rootdir is absolute and exists
	rootdir, err = filepath.Abs(rootdir)
	Ck(err)
	_, err = os.Stat(rootdir)
	Ck(err)
	// create the db
	g = &Grokker{
		Root:    rootdir,
		Version: version,
	}
	// initialize other bits
	err = g.setup(model)
	Ck(err)
	// ensure there is no existing db
	g.grokpath = filepath.Join(rootdir, name)
	_, err = os.Stat(g.grokpath)
	if err == nil {
		err = fmt.Errorf("db already exists at %q", g.grokpath)
		return
	}
	// save the db
	fh, err := os.Create(g.grokpath)
	Ck(err)
	err = g.Save()
	Ck(err)
	fh.Close()
	return
}

// Load loads a Grokker database from the current or any parent directory.
func Load() (g *Grokker, migrated bool, oldver, newver string, err error) {
	defer Return(&err)

	// find the .grok file in the current or any parent directory
	grokfnbase := ".grok"
	grokpath := ""
	for level := 0; level < 99; level++ {
		path := strings.Repeat("../", level) + grokfnbase
		if _, err := os.Stat(path); err == nil {
			grokpath = path
			break
		}
	}
	g, migrated, oldver, newver, err = LoadFrom(grokpath)
	Ck(err)
	return
}

// ListModels lists the available models.
func (g *Grokker) ListModels() (models []*Model, err error) {
	defer Return(&err)
	for _, model := range g.models.Available {
		models = append(models, model)
	}
	return
}

// SetModel sets the default chat completion model for queries.
func (g *Grokker) SetModel(model string) (oldModel string, err error) {
	defer Return(&err)
	model, _, err = g.models.findModel(model)
	Ck(err)
	oldModel, _, err = g.getModel()
	Ck(err)
	err = g.setup(model)
	Ck(err)
	return
}

// GitCommitMessage generates a git commit message given a diff. It
// appends a reasonable prompt, and then uses the result as a grokker
// query.
func (g *Grokker) GitCommitMessage(diff string) (msg string, err error) {
	defer Return(&err)

	// summarize the diff
	sumLines, msg, err := g.summarizeDiff(diff)
	Ck(err)

	// summarize the sumLines to create the first line of the commit
	// message
	resp, err := g.generate(SysMsgChat, GitSummaryPrompt, sumLines, false)
	Ck(err)
	summary := resp.Choices[0].Message.Content

	// glue it all together
	msg = Spf("%s\n\n%s", summary, msg)

	return
}

// TokenCount returns the number of tokens in a chunk, and caches the
// result in the chunk.
func (chunk *Chunk) TokenCount(g *Grokker) (count int, err error) {
	defer Return(&err)
	if chunk.tokenLength == 0 {
		text, err := g.chunkText(chunk, false)
		Ck(err)
		tokens, err := g.tokens(text)
		Ck(err)
		chunk.tokenLength = len(tokens)
	}
	count = chunk.tokenLength
	return
}

// Msg sends sysmsg and txt to openai and returns the response.
func (g *Grokker) Msg(sysmsg, txt string) (resp string, err error) {
	defer Return(&err)
	respmsg, err := g.msg(sysmsg, txt)
	Ck(err)
	resp = respmsg.Choices[0].Message.Content
	return
}
