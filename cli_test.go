package grokker

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	. "github.com/stevegt/goadapt"
)

// run executes the given command with the given arguments.
func run(t *testing.T, cmd string, args ...string) {
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	err := c.Run()
	Tassert(t, err == nil, "error running `%s`: %v", cmd, err)
}

// runOut executes the given command with the given arguments and returns the output.
func runOut(t *testing.T, cmd string, args ...string) string {
	c := exec.Command(cmd, args...)
	c.Stderr = os.Stderr
	out, err := c.Output()
	Tassert(t, err == nil, "error running `%s %s`: %v", cmd, args, err)
	return string(out)
}

// cd changes the current working directory to the given directory.
func cd(t *testing.T, dir string) {
	err := os.Chdir(dir)
	Tassert(t, err == nil, "error changing to directory %s: %v", dir, err)
}

// grok runs the grokker cli with the given arguments and returns
// stdout, stderr, rc, and err
func grok(stdin bytes.Buffer, args ...string) (stdout, stderr bytes.Buffer, err error) {
	defer Return(&err)
	// capture goadapt stdio
	SetStdio(&stdin, &stdout, &stderr)
	defer SetStdio(nil, nil, nil)

	// also pass stdio to the CLI
	config := NewConfig()
	config.Stdout = &stdout
	config.Stderr = &stderr

	// get the caller's filename and line number
	_, fn, line, _ := runtime.Caller(1)

	var exitRc int
	// replace the kong exit function with one that doesn't exit
	config.Exit = func(rc int) {
		if rc != 0 {
			msg := Spf("%s:%d rc: %v\nstderr:\n%s", fn, line, rc, stderr.String())
			fmt.Println(msg)
			exitRc = rc
		}
	}

	// run the CLI
	rc, err := Cli(args, config)
	if err == nil && (exitRc != 0 || rc != 0) {
		err = fmt.Errorf("rc: %v exitRc: %v", rc, exitRc)
	}
	return
}

func TestCli(t *testing.T) {
	// get current working directory
	cwd, err := os.Getwd()
	Tassert(t, err == nil, "error getting current working directory: %v", err)
	// create a temporary directory
	dir, err := os.MkdirTemp("", "grokker")
	Ck(err)
	defer os.RemoveAll(dir)
	// cd into the temporary directory
	cd(t, dir)
	defer cd(t, cwd)

	// create a stdin buffer
	var stdin bytes.Buffer

	// initialize a grokker repository
	stdout, stderr, err := grok(stdin, "init")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

	// pass in a command line that should work
	stdout, stderr, err = grok(stdin, "models")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match := strings.Contains(stdout.String(), "gpt-")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())
	// check that the stderr buffer is empty
	Tassert(t, stderr.String() == "", "CLI returned unexpected error: %s", stderr.String())

	// create a file
	f, err := os.Create("test.txt")
	Tassert(t, err == nil, "error creating file: %v", err)
	// put some content in the file
	_, err = f.WriteString("testing is good")

	// add the file to the repository
	stdout, stderr, err = grok(stdin, "add", "test.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

	// ask a question
	stdout, stderr, err = grok(stdin, "q", "is testing good? answer yes or no")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = strings.Contains(stdout.String(), "yes") || strings.Contains(stdout.String(), "Yes")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

}
