package main

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/shlex"
	. "github.com/stevegt/goadapt"
)

// Run runs a command in the shell, returning stdout, stderr, and rc
func Run(command string) (stdout, stderr []byte, rc int, err error) {
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
	// create a pipe for stdout
	stdoutPipe, err := cobj.StdoutPipe()
	Ck(err)
	// create a pipe for stderr
	stderrPipe, err := cobj.StderrPipe()
	Ck(err)
	// start the command
	err = cobj.Start()
	Ck(err)
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
	// generate a temporary filename for the prompt file
	dir := os.TempDir()
	fh, err := os.CreateTemp(dir, "*.txt")
	Ck(err)
	defer fh.Close()
	fn := fh.Name()
	for {
		loop(fn)
	}
}

func loop(promptfn string) {
	// git commit
	rc, err := RunInteractive("grok-commit -A")
	Ck(err)
	Assert(rc == 0, "grok-commit failed")

	// present user with an editor buffer where they can type a natural language instruction
	editor := os.Getenv("EDITOR")
	rc, err = RunInteractive(Spf("%s %s", editor, promptfn))
	Ck(err)
	Assert(rc == 0, "editor failed")

	inFns, err := getFiles()
	Pf("inFns: %v\n", inFns)

}

// getFiles returns a list of files to be processed
func getFiles() ([]string, error) {
	// send that along with all files to GPT API
	// get ignore list
	ignore := []string{}
	ignorefn := ".aidda/ignore"
	if _, err := os.Stat(ignorefn); err == nil {
		buf, err := ioutil.ReadFile(ignorefn)
		Ck(err)
		ignore = strings.Split(string(buf), "\n")
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
