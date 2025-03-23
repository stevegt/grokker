package aidda

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eiannone/keyboard"
	gitignore "github.com/sabhiram/go-gitignore"
	"github.com/stevegt/envi"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/client"
	"github.com/stevegt/grokker/v3/core"
	"github.com/stevegt/grokker/v3/util"
)

/*
XXX update this
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
	baseDir         string
	promptFn        string
	ignoreFn        string
	testFn          string
	generateStampFn string
	commitStampFn   string
	DefaultSysmsg   = "You are an expert Go programmer. Please make the requested changes to the given code or documentation."
	generateStamp   *Stamp
	commitStamp     *Stamp
)

// Stamp struct for handling timestamp files
type Stamp struct {
	filename string
}

// NewStamp creates a new Stamp instance
func NewStamp(filename string) *Stamp {
	return &Stamp{filename: filename}
}

// Create creates the timestamp file with the specified time.
// The file is zero-length, and its modification time is set to 't'.
func (s *Stamp) Create(t time.Time) error {
	file, err := os.OpenFile(s.filename, os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	file.Close()
	return os.Chtimes(s.filename, t, t)
}

// Ensure ensures the timestamp file exists. If it does not, it
// creates it with the given time.
func (s *Stamp) Ensure(t time.Time) error {
	if _, err := os.Stat(s.filename); os.IsNotExist(err) {
		return s.Create(t)
	}
	return nil
}

// Update updates the timestamp file's modification time to the current time.
func (s *Stamp) Update() error {
	now := time.Now()
	return os.Chtimes(s.filename, now, now)
}

// NewerThan returns true if our timestamp is newer than the modification time of the given file.
func (s *Stamp) NewerThan(fn string) (bool, error) {
	ourInfo, err := os.Stat(s.filename)
	// if our file does not exist, it is older
	if os.IsNotExist(err) {
		// create the file with an old timestamp
		err = s.Create(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC))
		return false, err
	}
	if err != nil {
		return false, fmt.Errorf("failed to stat %s: %v", s.filename, err)
	}
	theirInfo, err := os.Stat(fn)
	// if other file does not exist, it is older
	if os.IsNotExist(err) {
		// create the other file with an old timestamp
		other := NewStamp(fn)
		err = other.Create(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC))
		return true, err
	}
	if err != nil {
		return false, fmt.Errorf("failed to stat %s: %v", fn, err)
	}
	return ourInfo.ModTime().After(theirInfo.ModTime()), nil
}

// OlderThan returns true if our timestamp is older than the modification time of the given file.
func (s *Stamp) OlderThan(fn string) (bool, error) {
	ourInfo, err := os.Stat(s.filename)
	// if the file does not exist, it is newer
	if os.IsNotExist(err) {
		// create the file with a current timestamp
		err = s.Create(time.Now())
		return false, err
	}
	if err != nil {
		return false, fmt.Errorf("failed to stat %s: %v", s.filename, err)
	}
	theirInfo, err := os.Stat(fn)
	// if the other file does not exist, it is newer
	if os.IsNotExist(err) {
		// create the other file with a current timestamp
		other := NewStamp(fn)
		err = other.Create(time.Now())
		return true, err
	}
	if err != nil {
		return false, fmt.Errorf("failed to stat %s: %v", fn, err)
	}
	return ourInfo.ModTime().Before(theirInfo.ModTime()), nil
}

func Do(g *core.Grokker, modelName string, args ...string) (err error) {
	defer Return(&err)

	baseDir = g.Root

	// ensure we're in a git repository
	// XXX location might want to be more flexible
	_, err = os.Stat(Spf("%s/.git", baseDir))
	Ck(err)

	// XXX location might want to be more flexible
	dir := Spf("%s/.aidda", baseDir)

	// generate filenames
	// XXX these should all be in a struct
	promptFn = Spf("%s/prompt", dir)
	ignoreFn = Spf("%s/ignore", dir)
	testFn = Spf("%s/test", dir)
	generateStampFn = Spf("%s/generate.stamp", dir)
	commitStampFn = Spf("%s/commit.stamp", dir)

	// Initialize Stamp objects for generate and commit
	generateStamp = NewStamp(generateStampFn)
	commitStamp = NewStamp(commitStampFn)

	// Determine if interactive mode is active
	isInteractive := false
	for _, cmd := range args {
		if cmd == "menu" {
			isInteractive = true
			break
		}
	}

	// consume subcommands from args
	for len(args) > 0 {
		cmd := args[0]
		args = args[1:]
		Pl("aidda: running subcommand", cmd)
		switch cmd {
		case "init":
			err = initAidda(dir)
			Ck(err)
		case "menu":
			action, err := menu(g)
			Ck(err)
			// Push the selected action to the front of args
			args = append([]string{action}, args...)
		case "commit":
			// Check if prompt is newer than generate.stamp
			promptIsNewer, err := generateStamp.OlderThan(promptFn)
			Ck(err)
			if promptIsNewer {
				Pl("Prompt has been updated since the last generation.")
				Pl("Please run 'grok aidda regenerate' or 'grok aidda force-commit'")
				if isInteractive {
					// Push 'menu' to the front of args to redisplay the menu
					args = append([]string{"menu"}, args...)
					continue
				} else {
					return fmt.Errorf("prompt has been updated since the last generation")
				}
			}
			// commit using current prompt as commit message
			var p *Prompt
			p, err = getPrompt(promptFn)
			Ck(err)
			err = commit(g, p.Txt)
			Ck(err)
		case "generate":
			// Check if generate.stamp is newer than commit.stamp
			genIsNewer, err := generateStamp.NewerThan(commitStampFn)
			Ck(err)
			if genIsNewer {
				Pl("generate.stamp is newer than commit.stamp")
				Pl("Please run 'grok aidda regenerate'")
				if isInteractive {
					// Push 'menu' to the front of args to redisplay the menu
					args = append([]string{"menu"}, args...)
					continue
				} else {
					return fmt.Errorf("generate.stamp is newer than commit.stamp")
				}
			}
			// generate code from current prompt file contents
			var p *Prompt
			p, err = getPrompt(promptFn)
			Ck(err)
			err = generate(g, modelName, p)
			Ck(err)
		case "auto":
			// commit using the git diff to generate a commit message
			// do a git add -A first
			cmd := exec.Command("git", "add", "-A")
			out, err := cmd.CombinedOutput()
			Ck(err)
			Pl(string(out))
			// ...then generate the commit message
			summary, err := g.GitCommitMessage("--staged")
			Ck(err)
			Pl(summary)
			// ...then commit
			err = commit(g, summary)
			Ck(err)
			// then regenerate
			fallthrough
		case "regenerate":
			// Regenerate code from the prompt without committing
			var p *Prompt
			p, err = getPrompt(promptFn)
			Ck(err)
			err = generate(g, modelName, p)
			Ck(err)
		case "force-commit":
			// Commit using the current promptFn without checking
			var p *Prompt
			p, err = getPrompt(promptFn)
			Ck(err)
			err = commit(g, p.Txt)
			Ck(err)
		case "test":
			err = runTest(testFn)
			Ck(err)
		case "abort":
			// Abort the current operation
			Pl("Operation aborted by user.")
			return nil
		default:
			PrintUsageAndExit()
		}
	}

	return
}

func PrintUsageAndExit() {
	fmt.Println("Usage: go run main.go {subcommand ...}")
	fmt.Println("Subcommands:")
	fmt.Println("  menu          - Display the action menu")
	fmt.Println("  init          - Initialize the .aidda directory")
	fmt.Println("  commit        - Commit using the current prompt file contents as the commit message")
	fmt.Println("  generate      - Generate changes from GPT based on the prompt")
	fmt.Println("  regenerate    - Regenerate code from the prompt without committing")
	fmt.Println("  force-commit  - Commit changes without checking if the prompt has been updated")
	fmt.Println("  test          - Run tests and include the results in the next LLM prompt")
	fmt.Println("  abort         - Abort subcommand processing")
	os.Exit(1)
}

// Prompt is a struct that represents a prompt
type Prompt struct {
	Sysmsg string
	In     []string
	Out    []string
	Txt    string
}

// initAidda function is responsible for creating the .aidda directory and its contents
func initAidda(dir string) (err error) {
	defer Return(&err)

	// create a directory for aidda files
	err = os.MkdirAll(dir, 0755)
	Ck(err)

	// Ensure there is an ignore file
	err = ensureIgnoreFile(ignoreFn)
	Ck(err)

	// Ensure timestamp files exist
	now := time.Now()
	err = generateStamp.Ensure(now)
	Ck(err)
	err = commitStamp.Ensure(now)
	Ck(err)

	// Check if the file exists
	_, err = os.Stat(promptFn)
	if os.IsNotExist(err) {
		err = createPromptFile(promptFn)
		Ck(err)
	} else {
		Ck(err)
	}
	return
}

// TODO: The getPrompt function not only retrieves the prompt but also handles user interaction.
// Refactoring this function to separate user interaction from prompt retrieval would improve its clarity
// and make it easier to test.

// getPrompt retrieves the prompt without creating it.
func getPrompt(promptFn string) (p *Prompt, err error) {
	defer Return(&err)

	// If AIDDA_EDITOR is set, open the editor where the users can
	// type a natural language instruction
	editor := envi.String("AIDDA_EDITOR", "")
	if editor != "" {
		Pf("Opening editor %s\n", editor)
		rc, err := RunInteractive(Spf("%s %s", editor, promptFn))
		Ck(err)
		Assert(rc == 0, "editor failed")
	}

	p, err = readPrompt(promptFn)
	Ck(err)

	return p, err
}

// TODO: The readPrompt function handles both parsing the prompt file and expanding file paths.
// Splitting these concerns into separate functions would make the code more modular and easier to maintain.

// readPrompt reads a prompt file
func readPrompt(path string) (p *Prompt, err error) {
	defer Return(&err)
	p = &Prompt{}

	// Read entire content of the file
	rawBuf, err := os.ReadFile(path)
	Ck(err)
	// Process directives
	// Lines that start with . are directives
	lines := []string{}
	rawLines := strings.Split(string(rawBuf), "\n")
	for i, line := range rawLines {
		// Ensure the first line doesn't start with a # (the default
		// prompt file starts with a comment; we want to make sure
		// the user edits it)
		if i == 0 && strings.HasPrefix(line, "#") {
			return nil, fmt.Errorf("prompt file must not start with a comment")
		}

		// Ensure there is a blank line after the first line
		if i == 1 {
			// trim leading and trailing whitespace
			trimmedLine := strings.TrimSpace(line)
			if trimmedLine != "" {
				// spew.Dump(line)
				return nil, fmt.Errorf("prompt file must have a blank line after the first line, just like a commit message")
			}
		}

		// .stop directive stops reading the prompt file
		if strings.HasPrefix(line, ".stop") {
			break
		}

		lines = append(lines, line)
	}
	// Remove empty lines at the end
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	// Find the index where headers start
	hdrStart := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			// empty line means headers start after this line
			break
		}
		if strings.Contains(line, ":") {
			// Found a header
			hdrStart = i
		} else {
			// continuation line
			continue
		}
	}

	if hdrStart >= len(lines) {
		return nil, fmt.Errorf("no headers found at the end of the prompt file")
	}

	// Extract headers
	headers := lines[hdrStart:]
	headerMap, err := extractHeaders(headers)
	if err != nil {
		return nil, err
	}

	// Use the prompt text excluding headers as the prompt and commit message
	p.Txt = strings.Join(lines[:hdrStart], "\n")
	// Pl(p.Txt)

	// Process headers
	err = processHeaders(headerMap, path, p)
	if err != nil {
		return nil, err
	}

	return p, nil
}

// processHeaders processes the header map and sets the Prompt fields accordingly
func processHeaders(headerMap map[string]string, path string, p *Prompt) error {
	p.Sysmsg = strings.TrimSpace(headerMap["Sysmsg"])
	inStr := strings.TrimSpace(headerMap["In"])
	outStr := strings.TrimSpace(headerMap["Out"])

	// Filenames are space-separated
	p.In = strings.Fields(inStr)
	p.Out = strings.Fields(outStr)

	// Files are relative to the parent of the .aidda directory
	// unless they are absolute paths
	aiddaDir := filepath.Dir(path)
	parentDir := filepath.Dir(aiddaDir)

	// Convert p.In to absolute paths
	newIn := []string{}
	for _, f := range p.In {
		if f == "" {
			continue
		}
		if filepath.IsAbs(f) {
			newIn = append(newIn, f)
		} else {
			newIn = append(newIn, filepath.Join(parentDir, f))
		}
	}
	p.In = newIn

	// Similarly for p.Out
	newOut := []string{}
	for _, f := range p.Out {
		if f == "" {
			continue
		}
		if filepath.IsAbs(f) {
			newOut = append(newOut, f)
		} else {
			newOut = append(newOut, filepath.Join(parentDir, f))
		}
	}
	p.Out = newOut

	// If any input path is a directory, then replace it with the
	// list of files in that directory
	for i := 0; i < len(p.In); i++ {
		f := p.In[i]
		fi, err := os.Stat(f)
		if err != nil {
			return fmt.Errorf("error reading %s: %v", f, err)
		}
		if fi.IsDir() {
			files, err := getFilesInDir(f)
			if err != nil {
				return err
			}
			p.In = append(p.In[:i], append(files, p.In[i+1:]...)...)
			i += len(files) - 1
		}
	}

	return nil
}

// extractHeaders extracts headers from a slice of lines and returns a map
func extractHeaders(headers []string) (map[string]string, error) {
	headerMap := make(map[string]string)
	var currentKey string
	for _, h := range headers {
		if h == "" {
			continue
		}
		if strings.HasPrefix(h, " ") || strings.HasPrefix(h, "\t") {
			// Continuation line
			if currentKey == "" {
				return nil, fmt.Errorf("continuation line found without a preceding header")
			}
			continuation := strings.TrimSpace(h)
			headerMap[currentKey] += " " + continuation
		} else {
			parts := strings.SplitN(h, ":", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			currentKey = key
			headerMap[key] = value
		}
	}
	return headerMap, nil
}

// getFilesInDir returns a list of files in a directory
func getFilesInDir(dir string) (files []string, err error) {
	defer Return(&err)

	// Get ignore list
	ig, err := gitignore.CompileIgnoreFile(ignoreFn)
	Ck(err)

	files = []string{}
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		// If path is a directory, skip it
		if info.IsDir() {
			return nil
		}
		// Check if the file is in the ignore list
		if ig.MatchesPath(path) {
			return nil
		}
		// Only include regular files
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

	// Get the list of files to process
	inFns, err := getFiles()
	Ck(err)
	outFns := make([]string, len(inFns))
	copy(outFns, inFns)

	// Filenames are newline-separated, indented by 4 spaces
	inStr := strings.Join(inFns, "\n    ")
	outStr := strings.Join(outFns, "\n    ")

	// Write the initial prompt line and a blank line
	_, err = fmt.Fprintf(file, "# write commit message here -- it will be used as the LLM prompt\n\n")
	Ck(err)

	// Write the headers at the end
	_, err = fmt.Fprintf(file, "Sysmsg: %s\n", DefaultSysmsg)
	Ck(err)
	_, err = fmt.Fprintf(file, "In: %s\n", inStr)
	Ck(err)
	_, err = fmt.Fprintf(file, "Out: %s\n", outStr)
	Ck(err)

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
			// If others is empty, return the response without
			// checking candidates
			return
		}
		// Check if the response is in the list of candidates
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

	// Run go test -v
	stdout, stderr, _, _ := RunTee("go test -v")

	// Write test results to the file
	fh, err := os.Create(fn)
	Ck(err)
	_, err = fh.WriteString(Spf("\n\nstdout:\n%s\n\nstderr:%s\n\n", stdout, stderr))
	Ck(err)
	fh.Close()
	return err
}

func generate(g *core.Grokker, modelName string, p *Prompt) (err error) {
	defer Return(&err)

	prompt := p.Txt
	Pl(prompt)

	testResults, err := getTestResults(testFn, p.In, p.Out)
	Ck(err)
	if len(testResults) > 0 {
		Pl("Including test results in prompt")
		prompt = Spf("%s\n\n%s", p.Txt, testResults)
	}

	inFns := p.In
	outFns := p.Out
	var outFls []core.FileLang
	for _, fn := range outFns {
		lang, known, err := util.Ext2Lang(fn)
		if !known || err != nil {
			if lang != "" {
				Pf("Unknown language for file %s, defaulting to %s\n", fn, lang)
			} else {
				Pf("Unknown language for file %s, defaulting to empty\n", fn)
			}
		}
		outFls = append(outFls, core.FileLang{File: fn, Language: lang})
	}

	sysmsg := p.Sysmsg
	if sysmsg == "" {
		Pl("Sysmsg header missing, using default.")
		sysmsg = DefaultSysmsg
	}
	Pf("Sysmsg: %s\n", sysmsg)

	msgs := []client.ChatMsg{
		client.ChatMsg{Role: "USER", Content: prompt},
	}

	// Count tokens
	Pf("Token counts:\n")
	tcs := newTokenCounts(g)
	tcs.add("sysmsg", sysmsg)
	txt := ""
	for _, m := range msgs {
		txt += m.Content
	}
	tcs.add("msgs", txt)
	for _, f := range inFns {
		var buf []byte
		buf, err = os.ReadFile(f)
		Ck(err)
		txt = string(buf)
		tcs.add(f, txt)
	}
	tcs.showTokenCounts()

	Pl("Output files:")
	for _, f := range outFns {
		Pl(f)
	}

	Pl("Using model:", g.Model)

	Pf("Querying GPT...")
	// Start a goroutine to print dots while waiting for the response
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
	resp, err := g.SendWithFiles(modelName, sysmsg, msgs, inFns, outFls)
	Ck(err)
	elapsed := time.Since(start)
	stopDots <- true
	close(stopDots)
	Pf(" got response in %s\n", elapsed)

	// ExtractFiles(outFls, promptFrag, dryrun, extractToStdout)
	err = core.ExtractFiles(outFls, resp, false, false)
	Ck(err)

	// Write entire response to .aidda/response
	Assert(len(baseDir) > 0, "baseDir not set")
	respFn := Spf("%s/.aidda/response", baseDir)
	err = os.WriteFile(respFn, []byte(resp), 0644)
	Ck(err)

	// Update generate.stamp
	err = generateStamp.Update()
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
	// First find max width of name
	maxNameLen := 0
	for _, tc := range tcs.counts {
		if len(tc.name) > maxNameLen {
			maxNameLen = len(tc.name)
		}
	}
	// Then print the counts
	total := 0
	format := fmt.Sprintf("    %%-%ds: %%7d\n", maxNameLen)
	for _, tc := range tcs.counts {
		Pf(format, tc.name, tc.count)
		total += tc.count
	}
	// Then print the total
	Pf(format, "total", total)
}

func commit(g *core.Grokker, commitMsg string) (err error) {
	defer Return(&err)
	var rc int

	// Check git status for uncommitted changes
	stdout, stderr, rc, err := Run("git status --porcelain", nil)
	Ck(err)
	if len(stdout) > 0 {
		Pl(string(stdout))
		Pl(string(stderr))
		// git add
		rc, err = RunInteractive("git add -A")
		Assert(rc == 0, "git add failed")
		Ck(err)
		// git commit
		stdin := []byte(commitMsg)
		stdout, stderr, rc, err = Run("git commit -F -", stdin)
		Pl(string(stdout))
		Pl(string(stderr))
		Assert(rc == 0, "git commit failed")
		Ck(err)
	} else {
		Pl("Nothing to commit")
	}

	// Update commit.stamp
	err = commitStamp.Update()
	Ck(err)

	return err
}

// getFiles returns a list of files to be processed
func getFiles() (files []string, err error) {
	defer Return(&err)

	// Get ignore list
	ignoreFn := ".aidda/ignore"
	ig, err := gitignore.CompileIgnoreFile(ignoreFn)
	Ck(err)

	// Get list of files recursively
	files = []string{}
	err = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		// Ignore .git and .aidda directories
		if strings.Contains(path, ".git") || strings.Contains(path, ".aidda") {
			return nil
		}
		// Check if the file is in the ignore list
		if ig.MatchesPath(path) {
			return nil
		}
		// Skip non-files
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		// Add the file to the list
		files = append(files, path)
		return nil
	})
	Ck(err)
	return files, nil
}

// ensureIgnoreFile creates an ignore file if it doesn't exist
func ensureIgnoreFile(fn string) (err error) {
	defer Return(&err)
	// Check if the ignore file exists
	_, err = os.Stat(fn)
	if os.IsNotExist(err) {
		err = nil
		// Create the ignore file
		fh, err := os.Create(fn)
		Ck(err)
		defer fh.Close()
		// Write the default ignore patterns
		_, err = fh.WriteString(".git\n.idea\n.grok*\ngo.*\nnv.shada\n")
		Ck(err)
	}
	return err
}

// menu displays the action menu and returns the selected action as a string
func menu(g *core.Grokker) (action string, err error) {
	defer Return(&err)

	for {
		fmt.Println("Select an action:")
		fmt.Println("  [g]enerate      - Generate code from current prompt file contents")
		fmt.Println("  [r]egenerate    - Regenerate code from the prompt without committing")
		fmt.Println("  [c]ommit        - Commit using the current prompt file contents as the commit message")
		fmt.Println("  [f]orce-commit  - Commit changes even if only the prompt has been updated")
		fmt.Println("  [a]uto          - Auto-generate a commit message, commit, then regenerate code based on prompt")
		fmt.Println("  [t]est          - Run tests and include the results in the next LLM prompt")
		fmt.Println("  e[x]it          - Abort and exit the menu")
		fmt.Println("Press the corresponding key to select an action...")

		// Initialize keyboard
		if err := keyboard.Open(); err != nil {
			return "", fmt.Errorf("failed to open keyboard: %v", err)
		}
		defer keyboard.Close()

		// Wait for a single keypress
		char, _, err := keyboard.GetSingleKey()
		if err != nil {
			return "", fmt.Errorf("failed to get key: %v", err)
		}

		// Handle the keypress
		switch strings.ToLower(string(char)) {
		case "g":
			return "generate", nil
		case "r":
			return "regenerate", nil
		case "c":
			return "commit", nil
		case "f":
			return "force-commit", nil
		case "a":
			return "auto", nil
		case "t":
			return "test", nil
		case "x":
			return "abort", nil
		default:
			fmt.Printf("\nUnknown option: %s\n\n", string(char))
			// Continue the loop to re-display the menu
		}
	}
}

// getTestResults reads the test file and returns its contents if it
// exists and is newer than all input and output files.  If there are
// no new test results, then the test file is cleared and an empty
// string is returned.
func getTestResults(testFn string, inFns []string, outFns []string) (testResults string, err error) {
	// get the test file's modification time
	testStat, err := os.Stat(testFn)
	if os.IsNotExist(err) {
		err = nil
	} else {
		Ck(err)
		// Check if the test file is newer than any input or output files
		fns := append(inFns, outFns...)
		for _, fn := range fns {
			inStat, err := os.Stat(fn)
			// skip if the file does not exist
			if os.IsNotExist(err) {
				continue
			}
			Ck(err)
			if testStat.ModTime().After(inStat.ModTime()) {
				// return the test file contents
				buf, err := os.ReadFile(testFn)
				Ck(err)
				testResults = string(buf)
				break
			}
		}
	}
	if len(testResults) == 0 {
		// Clear the test file
		Pl("Clearing test file")
		err = os.WriteFile(testFn, []byte{}, 0644)
		Ck(err)
	}

	return testResults, err
}
