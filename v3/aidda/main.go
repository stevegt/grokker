package main

import (
	"bufio"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/shlex"
	"github.com/stevegt/envi"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/core"
)

// RunTee runs a command in the shell, with stdout and stderr tee'd to the terminal
func RunTee(command string) (stdout, stderr []byte, rc int, err error) {
	defer Return(&err)
	// shlex the command to get the command and args
	parts, err := shlex.Split(command)
	Ck(err)
	var args []string
	var cmd string
	if len(parts) > 1 {
		cmd = parts[0]
		args = parts[1:]
	} else {
		cmd = parts[0]
	}
	// create the command
	cobj := exec.Command(cmd, args...)
	// connect stdin to the terminal
	cobj.Stdin = os.Stdin

	// create a tee for stdout
	stdoutPipe, err := cobj.StdoutPipe()
	Ck(err)
	stdoutTee := io.TeeReader(stdoutPipe, os.Stdout)
	// create a tee for stderr
	stderrPipe, err := cobj.StderrPipe()
	Ck(err)
	stderrTee := io.TeeReader(stderrPipe, os.Stderr)
	// read the stdout in a goroutine
	go func() {
		stdout, err = ioutil.ReadAll(stdoutTee)
		Ck(err)
	}()

	// read the stderr in a goroutine
	go func() {
		stderr, err = ioutil.ReadAll(stderrTee)
		Ck(err)
	}()
	// wait for goroutines to get started
	// XXX use a waitgroup instead
	time.Sleep(100 * time.Millisecond)

	// start the command
	err = cobj.Start()
	Ck(err)
	// wait for the command to finish
	err = cobj.Wait()
	Ck(err)
	// get the return code
	rc = cobj.ProcessState.ExitCode()
	return
}

// Run runs a command in the shell, returning stdout, stderr, and rc
func Run(command string, stdin []byte) (stdout, stderr []byte, rc int, err error) {
	defer Return(&err)
	// shlex the command to get the command and args
	parts, err := shlex.Split(command)
	Ck(err)
	var args []string
	var cmd string
	if len(parts) > 1 {
		cmd = parts[0]
		args = parts[1:]
	} else {
		cmd = parts[0]
	}
	// create the command
	cobj := exec.Command(cmd, args...)
	// create a pipe for stdin
	stdinPipe, err := cobj.StdinPipe()
	Ck(err)
	// create a pipe for stdout
	stdoutPipe, err := cobj.StdoutPipe()
	Ck(err)
	// create a pipe for stderr
	stderrPipe, err := cobj.StderrPipe()
	Ck(err)
	// start the command
	err = cobj.Start()
	Ck(err)
	// pipe stdin to the command in a goroutine
	go func() {
		_, err = stdinPipe.Write(stdin)
		Ck(err)
		stdinPipe.Close()
	}()
	// read the stdout in a goroutine
	go func() {
		stdout, err = ioutil.ReadAll(stdoutPipe)
		Ck(err)
	}()
	// read the stderr in a goroutine
	go func() {
		stderr, err = ioutil.ReadAll(stderrPipe)
		Ck(err)
	}()
	// wait for the command to finish
	err = cobj.Wait()
	Ck(err)
	// get the return code
	rc = cobj.ProcessState.ExitCode()
	return
}

// RunInteractive runs a command in the shell, with stdio connected to the terminal
func RunInteractive(command string) (rc int, err error) {
	defer Return(&err)
	// shlex the command to get the command and args
	parts, err := shlex.Split(command)
	Ck(err)
	var args []string
	var cmd string
	if len(parts) > 1 {
		cmd = parts[0]
		args = parts[1:]
	} else {
		cmd = parts[0]
	}
	// create the command
	cobj := exec.Command(cmd, args...)
	// connect the stdio to the terminal
	cobj.Stdin = os.Stdin
	cobj.Stdout = os.Stdout
	cobj.Stderr = os.Stderr
	// start the command
	err = cobj.Start()
	Ck(err)
	// wait for the command to finish
	err = cobj.Wait()
	Ck(err)
	// get the return code
	rc = cobj.ProcessState.ExitCode()
	return
}

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

func main() {
	base := os.Args[1]
	err := os.Chdir(filepath.Dir(base))
	Ck(err)

	// ensure there is a .git directory
	_, err = os.Stat(".git")
	Ck(err)

	// ensure there is a .grok file
	_, err = os.Stat(".grok")
	Ck(err)

	// generate a temporary filename for the prompt file
	dir := Spf("%s/.aidda", filepath.Dir(base))
	err = os.MkdirAll(dir, 0755)
	Ck(err)
	fn := Spf("%s/prompt.md", dir)

	// open or create a grokker db
	g, lock, err := core.LoadOrInit(base, "gpt-4o")
	Ck(err)
	defer lock.Unlock()

	// loop forever
	done := false
	for !done {
		done, err = loop(g, fn)
		Ck(err)
		time.Sleep(3 * time.Second)
	}
}

func loop(g *core.Grokker, promptfn string) (done bool, err error) {
	defer Return(&err)
	var rc int
	// check git status for uncommitted changes
	stdout, stderr, rc, err := Run("git status --porcelain", nil)
	Ck(err)
	if len(stdout) > 0 {
		Pl(string(stdout))
		Pl(string(stderr))
		// git add
		rc, err = RunInteractive("git add -A")
		Assert(rc == 0, "git add failed")
		Ck(err)
		// generate a commit message
		summary, err := g.GitCommitMessage("--staged")
		Ck(err)
		// git commit
		stdout, stderr, rc, err := Run("git commit -F-", []byte(summary))
		Assert(rc == 0, "git commit failed")
		Ck(err)
		Pl(string(stdout))
		Pl(string(stderr))
	}

	// present user with an editor buffer where they can type a natural language instruction
	editor := envi.String("EDITOR", "vim")
	rc, err = RunInteractive(Spf("%s %s", editor, promptfn))
	Ck(err)
	Assert(rc == 0, "editor failed")
	buf, err := ioutil.ReadFile(promptfn)
	Ck(err)
	prompt := string(buf)

	inFns, err := getFiles()

	Pf("inFns: %v\n", inFns)

	outFls := []core.FileLang{
		core.FileLang{File: "main.go", Language: "go"},
		core.FileLang{File: "main_test.go", Language: "go"},
	}
	// outFileRe := core.OutfilesRegex(outFls)

	sysmsg := "You are an expert Go programmer. Please make the requested changes to the given code."
	msgs := []core.ChatMsg{
		core.ChatMsg{Role: "USER", Txt: prompt},
	}

	resp, err := g.SendWithFiles(sysmsg, msgs, inFns, outFls)
	Ck(err)

	// ExtractFiles(outFls, promptFrag, dryrun, extractToStdout)
	err = core.ExtractFiles(outFls, resp, false, false)
	Ck(err)

	// run difftool
	difftool := envi.String("DIFFTOOL", "git difftool")
	rc, err = RunInteractive(difftool)
	Ck(err)
	Assert(rc == 0, "difftool failed")

	// run go test -v
	stdout, stderr, _, _ = RunTee("go test -v")

	// ask if they want to continue
	Pf("Enter to continue, or anything else to quit: ")
	// get user input
	reader := bufio.NewReader(os.Stdin)
	text, _ := reader.ReadString('\n')
	text = strings.TrimSpace(text)
	if text != "" {
		done = true
		return
	}

	// append test results to the prompt file
	fh, err := os.OpenFile(promptfn, os.O_APPEND|os.O_WRONLY, 0644)
	Ck(err)
	_, err = fh.WriteString(Spf("\n\nstdout:\n%s\n\nstderr:%s\n\n", stdout, stderr))
	Ck(err)
	fh.Close()

	return
}

// getFiles returns a list of files to be processed
func getFiles() ([]string, error) {
	// send that along with all files to GPT API
	// get ignore list
	ignore := []string{}
	ignorefn := ".aidda/ignore"
	if _, err := os.Stat(ignorefn); err == nil {
		// open the ignore file
		fh, err := os.Open(ignorefn)
		Ck(err)
		defer fh.Close()
		// read the ignore file
		scanner := bufio.NewScanner(fh)
		for scanner.Scan() {
			// split the ignore file into a list of patterns, ignore blank
			// lines and lines starting with #
			line := scanner.Text()
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "#") {
				continue
			}
			ignore = append(ignore, line)
		}
	}

	// get list of files recursively
	files := []string{}
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		// ignore .git and .aidda directories
		if strings.Contains(path, ".git") || strings.Contains(path, ".aidda") {
			return nil
		}
		// check if the file is in the ignore list
		for _, pat := range ignore {
			re, err := regexp.Compile(pat)
			Ck(err)
			m := re.MatchString(path)
			if m {
				return nil
			}
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
