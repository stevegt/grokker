package x3

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"time"

	"github.com/google/shlex"
	. "github.com/stevegt/goadapt"
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
