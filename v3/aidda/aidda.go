package x3

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/emersion/go-message/mail"
	"github.com/fsnotify/fsnotify"
	gitignore "github.com/sabhiram/go-gitignore"
	"github.com/stevegt/envi"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/core"
	"github.com/stevegt/grokker/v3/util"
)

/*
- while true
	- git commit
	- present user with an editor buffer where they can type a natural language instruction
	- send that along with all files to GPT API
		- filter out files using .aidda/ignore
	- save returned files over top of the existing files
	- run 'git difftool' with vscode as in https://www.roboleary.net/vscode/2020/09/15/vscode-git.html
	- open diff tool in editor so user can selectively choose and edit changes
	- run go test -v
	- include test results in the prompt file
*/

func Do(cmd string) (err error) {
	defer Return(&err)

	base, err := os.Getwd()
	Ck(err)

	/*
		// ensure there is a .git directory
		_, err = os.Stat(".git")
		Ck(err)

		// ensure there is a .grok file
		_, err = os.Stat(".grok")
		Ck(err)
	*/

	// create a directory for aidda files
	dir := Spf("%s/.aidda", base)
	err = os.MkdirAll(dir, 0755)
	Ck(err)

	// open or create a grokker db
	g, lock, err := core.LoadOrInit(base, "gpt-4o")
	Ck(err)
	defer lock.Unlock()

	// generate filenames
	promptFn := Spf("%s/prompt", dir)
	ignoreFn := Spf("%s/ignore", dir)

	// ensure there is an ignore file
	err = ensureIgnoreFile(ignoreFn)
	Ck(err)

	Pl("aidda: running subcommand", cmd)
	switch cmd {
	case "commit":
		// commit the current state
		err = commit(g)
		Ck(err)
	case "prompt":
		p, err := getPrompt(promptFn)
		Ck(err)
		spew.Dump(p)
		err = getChanges(g, p)
		Ck(err)
	case "diff":
		err = runDiff()
		Ck(err)
	case "test":
		err = runTest(promptFn)
		Ck(err)
	default:
		Assert(false, Spf("unknown command %s", cmd))
	}

	return
}

// Prompt is a struct that represents a prompt
type Prompt struct {
	In  []string
	Out []string
	Txt string
}

// NewPrompt opens or creates a prompt object
func NewPrompt(path string) (p *Prompt, err error) {
	defer Return(&err)
	// check if the file exists
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		err = createPromptFile(path)
		Ck(err)
	} else {
		Ck(err)
	}
	p, err = readPrompt(path)
	Ck(err)
	return
}

// readPrompt reads a prompt file
func readPrompt(path string) (p *Prompt, err error) {
	p = &Prompt{}
	// parse the file as a mail message
	file, err := os.Open(path)
	Ck(err)
	defer file.Close()
	mr, err := mail.CreateReader(file)
	Ck(err)
	// read the message header
	header := mr.Header
	inStr := header.Get("In")
	outStr := header.Get("Out")
	p.In = strings.Split(inStr, ", ")
	p.Out = strings.Split(outStr, ", ")
	// read the message body
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		Ck(err)
		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			// prompt text is in the body
			buf, err := io.ReadAll(part.Body)
			Ck(err)
			// trim leading and trailing whitespace
			txt := strings.TrimSpace(string(buf))
			p.Txt = string(txt)
		case *mail.AttachmentHeader:
			// XXX keep this here because we might perhaps use
			// attachments in the future for e.g. test results
			filename, err := h.Filename()
			Ck(err)
			fmt.Printf("Got attachment: %v\n", filename)
		}
	}
	return
}

// createPromptFile creates a new prompt file
func createPromptFile(path string) (err error) {
	defer Return(&err)
	file, err := os.Create(path)
	Ck(err)
	defer file.Close()

	// get the list of files to process
	inFns, err := getFiles()
	outFns := inFns[:]
	inStr := strings.Join(inFns, ", ")
	outStr := strings.Join(outFns, ", ")

	// create headers
	hmap := map[string][]string{
		"In":  []string{inStr},
		"Out": []string{outStr},
	}
	h := mail.HeaderFromMap(hmap)

	// create mail writer
	mw, err := mail.CreateSingleInlineWriter(file, h)
	Ck(err)
	// Write the body
	io.WriteString(mw, "# enter prompt here")

	return
}

// ask asks the user a question and gets a response
func ask(question, deflt string, others ...string) (response string, err error) {
	defer Return(&err)
	var candidates []string
	candidates = append(candidates, strings.ToUpper(deflt))
	for _, o := range others {
		candidates = append(candidates, strings.ToLower(o))
	}
	for {
		fmt.Printf("%s [%s]: ", question, strings.Join(candidates, "/"))
		reader := bufio.NewReader(os.Stdin)
		response, err = reader.ReadString('\n')
		Ck(err)
		response = strings.TrimSpace(response)
		if response == "" {
			response = deflt
		}
		if len(others) == 0 {
			// if others is empty, return the response without
			// checking candidates
			return
		}
		// check if the response is in the list of candidates
		for _, c := range candidates {
			if strings.ToLower(response) == strings.ToLower(c) {
				return
			}
		}
	}
}

func runTest(promptFn string) (err error) {
	defer Return(&err)
	Pf("Running tests\n")

	// run go test -v
	stdout, stderr, _, _ := RunTee("go test -v")

	// append test results to the prompt file
	fh, err := os.OpenFile(promptFn, os.O_APPEND|os.O_WRONLY, 0644)
	Ck(err)
	_, err = fh.WriteString(Spf("\n\nstdout:\n%s\n\nstderr:%s\n\n", stdout, stderr))
	Ck(err)
	fh.Close()
	return err
}

func runDiff() (err error) {
	defer Return(&err)
	// run difftool
	difftool := envi.String("AIDDA_DIFFTOOL", "git difftool")
	Pf("Running difftool %s\n", difftool)
	var rc int
	rc, err = RunInteractive(difftool)
	Ck(err)
	Assert(rc == 0, "difftool failed")
	return err
}

func getChanges(g *core.Grokker, p *Prompt) (err error) {
	defer Return(&err)
	Pf("getting changes from GPT\n")

	prompt := p.Txt
	inFns := p.In
	outFns := p.Out
	var outFls []core.FileLang
	for _, fn := range outFns {
		lang, known, err := util.Ext2Lang(fn)
		Ck(err)
		if !known {
			Pf("Unknown language for file %s, defaulting to text\n", fn)
			lang = "text"
		}
		outFls = append(outFls, core.FileLang{File: fn, Language: lang})
	}

	sysmsg := "You are an expert Go programmer. Please make the requested changes to the given code."
	msgs := []core.ChatMsg{
		core.ChatMsg{Role: "USER", Txt: prompt},
	}

	resp, err := g.SendWithFiles(sysmsg, msgs, inFns, outFls)
	Ck(err)

	// ExtractFiles(outFls, promptFrag, dryrun, extractToStdout)
	err = core.ExtractFiles(outFls, resp, false, false)
	Ck(err)

	return
}

func getPrompt(promptFn string) (p *Prompt, err error) {
	defer Return(&err)
	var rc int

	// read or create the prompt file
	p, err = NewPrompt(promptFn)
	Ck(err)

	// create fsnotify watcher
	watcher, err := fsnotify.NewWatcher()
	Ck(err)
	defer watcher.Close()
	// watch the prompt file
	err = watcher.Add(promptFn)
	Ck(err)

	// if AIDDA_EDITOR is set, open the editor where the users can
	// type a natural language instruction
	editor := envi.String("AIDDA_EDITOR", "")
	if editor != "" {
		Pf("Opening editor %s\n", editor)
		rc, err = RunInteractive(Spf("%s %s", editor, promptFn))
		Ck(err)
		Assert(rc == 0, "editor failed")
	}
	if false {
		// wait for the file to be saved
		Pf("Waiting for file %s to be saved\n", promptFn)
		err = waitForFile(watcher, promptFn)
		Ck(err)
	}

	// re-read the prompt file
	p, err = NewPrompt(promptFn)
	Ck(err)

	return p, err
}

func commit(g *core.Grokker) (err error) {
	defer Return(&err)
	var rc int
	// check git status for uncommitted changes
	stdout, stderr, rc, err := Run("git status --porcelain", nil)
	Ck(err)
	if len(stdout) > 0 {
		Pl(string(stdout))
		Pl(string(stderr))
		// res, err := ask("There are uncommitted changes. Commit?", "y", "n")
		// Ck(err)
		// if res == "y" {
		if true {
			// git add
			rc, err = RunInteractive("git add -A")
			Assert(rc == 0, "git add failed")
			Ck(err)
			// generate a commit message
			summary, err := g.GitCommitMessage("--staged")
			Ck(err)
			Pl(summary)
			// git commit
			stdout, stderr, rc, err := Run("git commit -F-", []byte(summary))
			Assert(rc == 0, "git commit failed")
			Ck(err)
			Pl(string(stdout))
			Pl(string(stderr))
		}
	}
	return err
}

// getFiles returns a list of files to be processed
func getFiles() (files []string, err error) {
	defer Return(&err)

	// get ignore list
	ignoreFn := ".aidda/ignore"
	ig, err := gitignore.CompileIgnoreFile(ignoreFn)
	Ck(err)

	// get list of files recursively
	files = []string{}
	err = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		// ignore .git and .aidda directories
		if strings.Contains(path, ".git") || strings.Contains(path, ".aidda") {
			return nil
		}
		// check if the file is in the ignore list
		if ig.MatchesPath(path) {
			return nil
		}
		// skip non-files
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		// add the file to the list
		files = append(files, path)
		return nil
	})
	Ck(err)
	return files, nil
}

// waitForFile waits for a file to be saved
func waitForFile(watcher *fsnotify.Watcher, fn string) (err error) {
	defer Return(&err)
	// wait for the file to be saved
	for {
		select {
		case event, ok := <-watcher.Events:
			Assert(ok, "watcher.Events closed")
			Pf("event: %v\n", event)
			write := event.Op&fsnotify.Write == fsnotify.Write
			create := event.Op&fsnotify.Create == fsnotify.Create
			rename := event.Op&fsnotify.Rename == fsnotify.Rename
			if write || create || rename {
				Pf("modified file: %s\n", event.Name)
				// check if absolute path of the file is the same as the
				// file we are waiting for
				if filepath.Clean(event.Name) == filepath.Clean(fn) {
					Pf("file %s written to\n", fn)
					// wait for writes to finish
					time.Sleep(1 * time.Second)
					return
				}
			}
		case err, ok := <-watcher.Errors:
			Assert(ok, "watcher.Errors closed")
			return err
		}
	}
}

// ensureIgnoreFile creates an ignore file if it doesn't exist
func ensureIgnoreFile(fn string) (err error) {
	defer Return(&err)
	// check if the ignore file exists
	_, err = os.Stat(fn)
	if os.IsNotExist(err) {
		err = nil
		// create the ignore file
		fh, err := os.Create(fn)
		Ck(err)
		defer fh.Close()
		// write the default ignore patterns
		_, err = fh.WriteString(".git\n.idea\n.grok*\ngo.*\nnv.shada\n")
		Ck(err)
	}
	return err
}
