package aidda

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	- include test results in the .aidda/test file
*/

var (
	baseDir  string
	ignoreFn string
)

var DefaultSysmsg = "You are an expert Go programmer. Please make the requested changes to the given code or documentation."

func Do(g *core.Grokker, args ...string) (err error) {
	defer Return(&err)

	baseDir = g.Root

	// ensure we're in a git repository
	// XXX location might want to be more flexible
	_, err = os.Stat(Spf("%s/.git", baseDir))
	Ck(err)

	// create a directory for aidda files
	// XXX location might want to be more flexible
	dir := Spf("%s/.aidda", baseDir)
	err = os.MkdirAll(dir, 0755)
	Ck(err)

	// generate filenames
	// XXX these should all be in a struct
	promptFn := Spf("%s/prompt", dir)
	ignoreFn = Spf("%s/ignore", dir)
	testFn := Spf("%s/test", dir)

	// ensure there is an ignore file
	err = ensureIgnoreFile(ignoreFn)
	Ck(err)

	// create the prompt file if it doesn't exist
	_, err = NewPrompt(promptFn)
	Ck(err)

	// if the test file is newer than any input files, then include
	// the test results in the prompt, otherwise clear the test file
	testResults := ""
	testStat, err := os.Stat(testFn)
	if os.IsNotExist(err) {
		err = nil
	} else {
		Ck(err)
		// get the list of input files
		p, err := getPrompt(promptFn)
		Ck(err)
		inFns := p.In
		// check if the test file is newer than any input files
		for _, fn := range inFns {
			inStat, err := os.Stat(fn)
			Ck(err)
			if testStat.ModTime().After(inStat.ModTime()) {
				// include the test results in the prompt
				buf, err := ioutil.ReadFile(testFn)
				Ck(err)
				testResults = string(buf)
				break
			}
		}
	}
	if len(testResults) == 0 {
		// clear the test file
		Pl("Clearing test file")
		err = ioutil.WriteFile(testFn, []byte{}, 0644)
		Ck(err)
	}

	for i := 0; i < len(args); i++ {
		cmd := args[i]
		Pl("aidda: running subcommand", cmd)
		switch cmd {
		case "init":
			// already done by this point, so this is a no-op
		case "commit":
			// commit the current state
			err = commit(g)
			Ck(err)
		case "prompt":
			p, err := getPrompt(promptFn)
			Ck(err)
			// spew.Dump(p)
			err = getChanges(g, p, testResults)
			Ck(err)
		case "diff":
			err = runDiff()
			Ck(err)
		case "test":
			err = runTest(testFn)
			Ck(err)
		default:
			PrintUsageAndExit()
		}
	}

	return
}

func PrintUsageAndExit() {
	fmt.Println("Usage: go run main.go {subcommand ...}")
	fmt.Println("Subcommands:")
	fmt.Println("  commit  - Commit the current state")
	fmt.Println("  prompt  - Present the user with an editor to type a prompt and get changes from GPT")
	fmt.Println("  diff    - Run 'git difftool' to review changes")
	fmt.Println("  test    - Run tests and include the results in the prompt file")
	os.Exit(1)
}

// Prompt is a struct that represents a prompt
type Prompt struct {
	Sysmsg string
	In     []string
	Out    []string
	Txt    string
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
	// XXX move directive parsing to here
	// parse the file as a mail message
	rawFile, err := os.Open(path)
	Ck(err)
	defer rawFile.Close()
	// read rawFile into a buffer
	rawBuf, err := io.ReadAll(rawFile)
	Ck(err)
	// process directives
	// lines that start with . are directives
	cookedBuf := []byte{}
	lines := strings.Split(string(rawBuf), "\n")
	for _, line := range lines {

		// .stop directive stops reading the prompt file
		if strings.HasPrefix(line, ".stop") {
			break
		}

		/*
			.revise directive sets Out: equal to the first argument and
			prepends "Revise {outfile} to the prompt, e.g.:

			.revise foo.md

			...is replaced with this:

			```
			Out: foo.md

			Revise foo.md
			```
		*/
		if strings.HasPrefix(line, ".revise ") {
			path := strings.Fields(line)[1]
			outline := Spf("Out:\r\n    %s", path)
			reviseLine := Spf("Revise %s", path)
			txt := Spf("%s\r\n\r\n%s\r\n", outline, reviseLine)
			cookedBuf = append(cookedBuf, []byte(txt)...)
			continue
		}

		cookedBuf = append(cookedBuf, []byte(line+"\n")...)
	}
	Pl(string(cookedBuf))
	// create an io.Reader from the cooked buffer
	cookedFile := strings.NewReader(string(cookedBuf))
	// parse the file as a mail message
	mr, err := mail.CreateReader(cookedFile)
	Ck(err)
	// read the message header
	header := mr.Header
	inStr := header.Get("In")
	outStr := header.Get("Out")
	p.Sysmsg = header.Get("Sysmsg")
	// filenames are comma-separated
	// p.In = strings.Split(inStr, ", ")
	// p.Out = strings.Split(outStr, ", ")
	// filenames are space-separated
	p.In = strings.Fields(inStr)
	p.Out = strings.Fields(outStr)
	// files are relative to the parent of the .aidda directory
	// unless they are absolute paths
	aiddaDir := filepath.Dir(path)
	parentDir := filepath.Dir(aiddaDir)
	// convert to absolute paths, skip empty strings
	newIn := []string{}
	for _, f := range p.In {
		if filepath.IsAbs(f) {
			continue
		}
		if f == "" {
			continue
		}
		newIn = append(newIn, Spf("%s/%s", parentDir, f))
	}
	p.In = newIn
	newOut := []string{}
	for _, f := range p.Out {
		if filepath.IsAbs(f) {
			continue
		}
		if f == "" {
			continue
		}
		newOut = append(newOut, Spf("%s/%s", parentDir, f))
	}
	p.Out = newOut
	// if any input path is a directory, then replace it with the
	// list of files in that directory
	for i, f := range p.In {
		fi, err := os.Stat(f)
		Assert(err == nil, "error reading %s: %v", f, err)
		if fi.IsDir() {
			files, err := getFilesInDir(f)
			Ck(err)
			p.In = append(p.In[:i], append(files, p.In[i+1:]...)...)
		}
	}
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
			txt1 := strings.TrimSpace(string(buf))
			// body text lines that start with . are directives
			// XXX move directive parsing to happen before parsing the headers
			lines := strings.Split(txt1, "\n")
			txt2 := ""
			for _, line := range lines {
				// .stop directive stops reading the prompt file
				if strings.HasPrefix(line, ".stop") {
					break
				}
				txt2 += line + "\n"
			}
			p.Txt = txt2
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

// getFilesInDir returns a list of files in a directory
func getFilesInDir(dir string) (files []string, err error) {
	defer Return(&err)

	// get ignore list
	ig, err := gitignore.CompileIgnoreFile(ignoreFn)
	Ck(err)

	files = []string{}
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		// if path is a directory, skip it
		if info.IsDir() {
			return nil
		}
		// check if the file is in the ignore list
		if ig.MatchesPath(path) {
			return nil
		}
		// only include regular files
		if !info.Mode().IsRegular() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
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
	// filenames are comma-separated
	// inStr := strings.Join(inFns, ", ")
	// outStr := strings.Join(outFns, ", ")
	// filenames are space-separated
	inStr := strings.Join(inFns, " ")
	outStr := strings.Join(outFns, " ")

	// create headers
	hmap := map[string][]string{
		"Sysmsg": []string{DefaultSysmsg},
		"In":     []string{inStr},
		"Out":    []string{outStr},
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

func runTest(fn string) (err error) {
	defer Return(&err)
	Pf("Running tests\n")

	// run go test -v
	stdout, stderr, _, _ := RunTee("go test -v")

	// write test results to the file
	fh, err := os.Create(fn)
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

func getChanges(g *core.Grokker, p *Prompt, testResults string) (err error) {
	defer Return(&err)

	if len(testResults) > 0 {
		Pl("Including test results in prompt")
	}
	prompt := Spf("%s\n\n%s", p.Txt, testResults)
	inFns := p.In
	outFns := p.Out
	var outFls []core.FileLang
	for _, fn := range outFns {
		lang, known, err := util.Ext2Lang(fn)
		Ck(err)
		if !known {
			Pf("Unknown language for file %s, defaulting to %s\n", fn, lang)
		}
		outFls = append(outFls, core.FileLang{File: fn, Language: lang})
	}

	sysmsg := p.Sysmsg
	if sysmsg == "" {
		Pf("Sysmsg header missing, using default.")
		sysmsg = DefaultSysmsg
	}
	Pf("Sysmsg: %s\n", sysmsg)

	msgs := []core.ChatMsg{
		core.ChatMsg{Role: "USER", Txt: prompt},
	}

	// count tokens
	Pf("Token counts:\n")
	tcs := newTokenCounts(g)
	tcs.add("sysmsg", sysmsg)
	txt := ""
	for _, m := range msgs {
		txt += m.Txt
	}
	tcs.add("msgs", txt)
	for _, f := range inFns {
		var buf []byte
		buf, err = ioutil.ReadFile(f)
		Ck(err)
		txt = string(buf)
		tcs.add(f, txt)
	}
	tcs.showTokenCounts()

	Pl("Output files:")
	for _, f := range outFns {
		Pl(f)
	}

	Pf("Querying GPT...")
	// start a goroutine to print dots while waiting for the response
	var stopDots = make(chan bool)
	go func() {
		for {
			select {
			case <-stopDots:
				return
			default:
				time.Sleep(1 * time.Second)
				fmt.Print(".")
			}
		}
	}()
	start := time.Now()
	resp, err := g.SendWithFiles(sysmsg, msgs, inFns, outFls)
	Ck(err)
	elapsed := time.Since(start)
	stopDots <- true
	close(stopDots)
	Pf(" got response in %s\n", elapsed)

	// ExtractFiles(outFls, promptFrag, dryrun, extractToStdout)
	err = core.ExtractFiles(outFls, resp, false, false)
	Ck(err)

	// write entire response to .addda/response
	Assert(len(baseDir) > 0, "baseDir not set")
	respFn := Spf("%s/.aidda/response", baseDir)
	err = ioutil.WriteFile(respFn, []byte(resp), 0644)
	Ck(err)

	return
}

type tokenCount struct {
	name  string
	text  string
	count int
}

type tokenCounts struct {
	g      *core.Grokker
	counts []tokenCount
}

// newTokenCounts creates a new tokenCounts object
func newTokenCounts(g *core.Grokker) *tokenCounts {
	return &tokenCounts{g: g}
}

// add adds a token count to a tokenCounts object
func (tcs *tokenCounts) add(name, text string) {
	count, err := tcs.g.TokenCount(text)
	Ck(err)
	tc := tokenCount{name: name, text: text, count: count}
	tcs.counts = append(tcs.counts, tc)
	return
}

// showTokenCounts shows the token counts for a slice of tokenCount
func (tcs *tokenCounts) showTokenCounts() {
	// first find max width of name
	maxNameLen := 0
	for _, tc := range tcs.counts {
		if len(tc.name) > maxNameLen {
			maxNameLen = len(tc.name)
		}
	}
	// then print the counts
	total := 0
	format := fmt.Sprintf("    %%-%ds: %%7d\n", maxNameLen)
	for _, tc := range tcs.counts {
		Pf(format, tc.name, tc.count)
		total += tc.count
	}
	// then print the total
	Pf(format, "total", total)
}

func getPrompt(promptFn string) (p *Prompt, err error) {
	defer Return(&err)
	var rc int

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
