package core

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/client"
	"github.com/stevegt/grokker/v3/util"
	"github.com/stevegt/semver"
	"github.com/tiktoken-go/tokenizer"
)

// import "github.com/stevegt/grokker/v3/util"

// XXX move to api/api.go

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
		for {
			// try comparing the paths directly first.
			if d.RelPath == path {
				match = true
				break
			}
			// if that doesn't work, try comparing the absolute paths.
			relpath, err := filepath.Abs(path)
			Ck(err)
			if g.absPath(d) == relpath {
				match = true
				break
			}
			break
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

// Chat uses the given sysmsg and prompt along with context from the
// knowledge base and message history file to generate a response.
func (g *Grokker) Chat(modelName, sysmsg, prompt, fileName string, level util.ContextLevel, infiles []string, outfiles []FileLang, extract, promptTokenLimit int, extractToStdout, addToDb, edit bool) (resp string, err error) {
	defer Return(&err)
	// open the message history file
	history, err := g.OpenChatHistory(sysmsg, fileName)
	Ck(err)
	defer func() {
		err := history.Save(addToDb)
		Ck(err)
	}()
	if extract > 0 {
		// extract the Nth most recent files from the history
		err = history.extractFromChat(outfiles, extract, extractToStdout)
		Ck(err)
		return
	}
	// get response
	resp, _, err = history.ContinueChat(modelName, prompt, level, infiles, outfiles, promptTokenLimit, edit)
	Ck(err)
	return
}

// Context returns the context for a given text, limited by the
// tokenLimit.
func (g *Grokker) Context(text string, tokenLimit int, withHeaders, withLineNumbers bool) (context string, err error) {
	defer Return(&err)
	// call getContext() with the tokenLimit
	context, err = g.getContext(text, tokenLimit, withHeaders, withLineNumbers, nil)
	return
}

// Continue returns a continuation of the input text.
func (g *Grokker) Continue(modelName, in string, global bool) (out, sysmsg string, err error) {
	defer Return(&err)
	sysmsg = SysMsgContinue
	// tokenize sysmsg
	_, sysmsgTokens, err := Tokenizer.Encode(sysmsg)
	Ck(err)
	// tokenize input
	_, inTokens, err := Tokenizer.Encode(in)
	Ck(err)
	// get chunks, sorted by similarity to the txt.
	tokenLimit := int(float64(g.ModelObj.TokenLimit)*0.4) - len(sysmsgTokens) - len(inTokens)
	context, err := g.getContext(in, tokenLimit, false, false, nil)
	Ck(err)
	// generate the answer.
	out, err = g.AnswerWithRAG(modelName, sysmsg, in, context, global)
	Ck(err)
	Debug("Continue() in: %s\ncontext: %s\nout: %s\n", in, context, out)
	return
}

// Answer returns the answer to a question.
func (g *Grokker) Answer(modelName, question string, withHeaders, withLineNumbers, global bool) (out string, err error) {
	defer Return(&err)
	// tokenize the question
	qtokens, err := g.tokens(question)
	Ck(err)
	maxTokens := int(float64(g.ModelObj.TokenLimit)*0.5) - len(qtokens)
	context, err := g.getContext(question, maxTokens, withHeaders, withLineNumbers, nil)
	Ck(err)
	// generate the answer.
	out, err = g.AnswerWithRAG(modelName, SysMsgChat, question, context, global)
	return
}

// Revise returns revised text based on input text.
func (g *Grokker) Revise(modelName, in string, global, sysmsgin bool) (out, sysmsg string, err error) {
	defer Return(&err)

	// tokenize the entire input
	inTokens, err := g.tokens(in)

	if sysmsgin {
		// split input into sysmsg and txt
		paragraphs := strings.Split(in, "\n\n")
		if len(paragraphs) < 2 {
			err = fmt.Errorf("input must contain at least two paragraphs")
			return
		}
		sysmsg = paragraphs[0]
		in = strings.Join(paragraphs[1:], "\n\n")
	} else {
		sysmsg = SysMsgRevise
	}

	// get context
	maxTokens := int(float64(g.ModelObj.TokenLimit)*0.5) - len(inTokens)
	context, err := g.getContext(in, maxTokens, false, false, nil)
	Ck(err)

	// generate the answer.
	resp, err := g.AnswerWithRAG(modelName, sysmsg, in, context, global)
	Ck(err)
	if sysmsgin {
		out = Spf("%s\n\n%s", sysmsg, resp)
	} else {
		out = resp
	}

	Debug("Revise() in: %s\ncontext: %s\nout: %s\n", in, context, out)
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
	err = util.CopyFile(g.grokpath, backpath)
	Ck(err, "failed to backup %q to %q", g.grokpath, backpath)
	return
}

// Save saves the Grokker database to the stored path.
func (g *Grokker) Save() (err error) {
	defer Return(&err)
	err = g.saveToFile()
	Ck(err)
	return
}

// saveToFile handles the actual saving process
func (g *Grokker) saveToFile() (err error) {
	defer Return(&err)
	// open
	Debug("saving grok file")
	tmpfn := g.grokpath + ".tmp"
	fh, err := os.Create(tmpfn)
	Ck(err)
	// write
	data, err := json.Marshal(g)
	Ck(err)
	_, err = fh.Write(data)
	Ck(err)
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
	return Version
}

// DBVersion returns the version of the grokker database.
func (g *Grokker) DBVersion() string {
	return g.Version
}

// Embed returns the embedding for a given text as a JSON string.
func (g *Grokker) Embed(text string) (jsonEmbedding string, err error) {
	defer Return(&err)
	// call createEmbeddings() to get the embedding.
	embedding, err := g.createEmbeddings([]string{text})
	Ck(err)
	// convert the embedding to an indented JSON string.
	buf, err := json.MarshalIndent(embedding, "", "  ")
	Ck(err)
	jsonEmbedding = string(buf)
	return
}

// Similarity returns the similarity between two or more texts.  Each text
// is compared to the reference text, and the similarities are returned as
// a float64 slice.
func (g *Grokker) Similarity(reftext string, texts ...string) (sims []float64, err error) {
	defer Return(&err)
	// get the mean vector of the reference text
	refVec, err := g.meanVectorFromLongString(reftext)
	Ck(err)
	// compare each text to the reference text
	for _, text := range texts {
		// get the mean vector of the text
		vec, err := g.meanVectorFromLongString(text)
		Ck(err)
		// calculate the similarity
		sim := util.Similarity(refVec, vec)
		sims = append(sims, sim)
	}
	return
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
		cmp, err := semver.Cmp(current, v100)
		Ck(err)
		if cmp > 0 {
			path = doc.RelPath
		}
		paths = append(paths, path)
	}
	return
}

// LoadFrom loads a Grokker database from a given path.
// XXX replace the json db with a kv store, store vectors as binary
// floating point values.
func LoadFrom(grokpath string, newModel string, readonly bool) (g *Grokker, migrated bool, oldver, newver string, lock *flock.Flock, err error) {
	g = &Grokker{}
	g.grokpath = grokpath
	lockpath := grokpath + ".lock"
	// ensure the lock file exists
	lockfh, err := os.OpenFile(lockpath, os.O_CREATE, 0644)
	Ck(err)
	err = lockfh.Close()
	Ck(err)
	lock = flock.New(lockpath)
	if readonly {
		// get a shared lock
		Debug("locking %s ro...", lockpath)
		err = lock.RLock()
		Ck(err)
	} else {
		// get an exclusive lock
		lock = flock.New(lockpath)
		Debug("locking %s rw...", lockpath)
		err = lock.Lock()
		Ck(err)
	}
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

	// XXX this is janky -- we're getting the model from the db, but
	// then in Setup() we're setting it in the db.
	model := g.Model
	// replace the model if newModel is not empty
	if newModel != "" {
		model = newModel
	}

	err = g.Setup(model)
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
		Version: Version,
	}
	// initialize other bits
	err = g.Setup(model)
	Ck(err)
	// ensure there is no existing db
	g.grokpath = filepath.Join(rootdir, name)
	_, err = os.Stat(g.grokpath)
	if err == nil {
		err = fmt.Errorf("db already exists at %q", g.grokpath)
		return
	}
	// save the db
	// XXX why are we creating an empty file here?
	fh, err := os.Create(g.grokpath)
	Ck(err)
	// XXX let caller save the db instead of doing it here
	err = g.Save()
	Ck(err)
	fh.Close()
	return
}

// Load loads a Grokker database from the current or any parent directory.
func Load(newModel string, readonly bool) (g *Grokker, migrated bool, oldver, newver string, lock *flock.Flock, err error) {
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
	g, migrated, oldver, newver, lock, err = LoadFrom(grokpath, newModel, readonly)
	Ck(err)
	return
}

// ListModels lists the available models.
func (g *Grokker) ListModels() (models []*Model) {
	return g.models.ListModels()
}

// SetModel sets the default chat completion model for queries.
func (g *Grokker) SetModel(model string) (oldModel string, err error) {
	defer Return(&err)
	model, _, err = g.models.FindModel(model)
	Ck(err)
	oldModel, _, err = g.GetModel()
	Ck(err)
	err = g.Setup(model)
	Ck(err)
	return
}

// GitCommitMessage generates a git commit message given a diff. It
// appends a reasonable prompt, and then uses the result as a grokker
// query.
func (g *Grokker) GitCommitMessage(modelName string, args ...string) (msg string, err error) {
	defer Return(&err)

	Debug("GitCommitMessage(%s, %v)", modelName, args)

	// run `git diff @args
	args = append([]string{"diff"}, args...)
	cmd := exec.Command("git", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	Ck(err)
	diff := string(out)

	if false {
		// summarize the diff
		sumLines, msg, err := g.summarizeDiff(modelName, diff)
		Ck(err)

		// summarize the sumLines to create the first line of the commit
		// message
		// resp, err := g.generate(SysMsgChat, GitSummaryPrompt, sumLines, false)
		// Ck(err)
		_ = sumLines
		//
		// summarize the entire commit message to create the first line
		summary, err := g.AnswerWithRAG(modelName, SysMsgChat, GitSummaryPrompt, msg, false)
		Ck(err)

		// glue it all together
		msg = Spf("%s\n\n%s", summary, msg)
	}

	if true {
		// experimental: take advantage of more modern models that have
		// larger context windows and know what a commit message is
		sysmsg := "Write a git commit message for the given diff. Use present tense, active, imperative statements as if giving directions.  Do not use extra adjectives or marketing hype.  The first line of the commit message must be a summary of 60 characters or less, followed by a blank line, followed by bullet-pointed details.  Make a separate bullet list for each changed file."

		// Yes, we're giving the model the instructions twice -- once in the
		// sysmsg and once in the prompt.
		userMsg := Spf("%s\n\n%s", sysmsg, diff)

		msgs := []client.ChatMsg{
			client.ChatMsg{
				Role:    "User",
				Content: userMsg,
			},
		}

		res, _, err := g.CompleteChat(modelName, sysmsg, msgs)
		Ck(err)
		msg = res
	}

	return
}

// Msg sends sysmsg and txt to openai and returns the response.
func (g *Grokker) Msg(modelName, sysmsg, txt string) (out string, err error) {
	defer Return(&err)
	out, err = g.AnswerWithRAG(modelName, sysmsg, txt, "", false)
	return
}

// InitTokenizer initializes the tokenizer.
func InitTokenizer() (err error) {
	Tokenizer, err = tokenizer.Get(tokenizer.Cl100kBase)
	Ck(err)
	return
}

// LoadOrInit loads a Grokker database from the given path, or creates
// a new one if there is no database at the given path.
func XXXLoadOrInit(dir, model string) (g *Grokker, lock *flock.Flock, err error) {
	defer Return(&err)
	grokpath := filepath.Join(dir, ".grok")
	_, err = os.Stat(grokpath)
	if os.IsNotExist(err) {
		Pf("creating new database at %s\n", grokpath)
		_, err = Init(dir, model)
		Ck(err)
	}
	Pf("loading database from %s\n", grokpath)
	g, _, _, _, lock, err = LoadFrom(grokpath, "", false)
	Ck(err)
	Pf("loaded database from %s\n", grokpath)
	return
}
