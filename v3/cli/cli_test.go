package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stevegt/grokker/v3/core"

	"github.com/sergi/go-diff/diffmatchpatch"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/util"
)

// dumpDiff returns a pretty-printed diff between two byte slices
func dumpDiff(bufA, bufB []byte) string {
	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(string(bufA), string(bufB), false)
	return fmt.Sprintf("%s\n", dmp.DiffPrettyText(diffs))
}

// hashFile returns the SHA256 hash of the given file
func hashFile(filename string) (digest string, err error) {
	defer Return(&err)
	// Open the file for reading
	file, err := os.Open(filename)
	Ck(err)
	defer file.Close()
	digest, err = hashReader(file)
	Ck(err)
	return
}

// hashBytes returns the SHA256 hash of the given byte slice
func hashBytes(buf []byte) (digest string, err error) {
	defer Return(&err)
	// create a reader from the byte slice
	reader := bytes.NewReader(buf)
	digest, err = hashReader(reader)
	Ck(err)
	return
}

// hashReader returns the SHA256 hash of the given io.Reader
func hashReader(file io.Reader) (digest string, err error) {
	defer Return(&err)
	// Create a new SHA256 hash.Hash object
	hash := sha256.New()
	// Copy the file content to the hash interface writer
	_, err = io.Copy(hash, file)
	Ck(err)
	// Summarize the hash into a final hash result
	// then convert it to a hexadecimal string.
	buf := hash.Sum(nil)
	digest = hex.EncodeToString(buf)
	return
}

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

	// pass stdio to the CLI
	config := NewCliConfig()
	config.Stdin = &stdin
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

// mkFile creates a file with the given name and content.
func mkFile(t *testing.T, name, content string) {
	f, err := os.Create(name)
	Tassert(t, err == nil, "error creating file: %v", err)
	_, err = f.WriteString(content)
	Tassert(t, err == nil, "error writing to file: %v", err)
	err = f.Close()
	Tassert(t, err == nil, "error closing file: %v", err)
}

// cimatch returns true if the given string contains the given
// substring, ignoring case.
func cimatch(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

func TestCli(t *testing.T) {

	var stdout, stderr bytes.Buffer
	var emptyStdin bytes.Buffer
	var match bool
	var err error

	// test stringInSlice
	// XXX move to util_test.go
	slice := []string{"msg", "tc"}
	Tassert(t, util.StringInSlice("msg", slice), "stringInSlice failed")
	Tassert(t, !util.StringInSlice("msg2", slice), "stringInSlice failed")

	// test similarity subcommand
	fmt.Println("testing similarity...")
	// test with an empty file
	stdout, stderr, err = grok(emptyStdin, "similarity", "testdata/sim1.md", "/dev/null")
	if err != nil {
		cwd, err := os.Getwd()
		Ck(err)
		t.Logf("current working directory: %s", cwd)
		t.Logf("stdout: %s", stdout.String())
		t.Logf("stderr: %s", stderr.String())
		t.Fatalf("CLI returned unexpected error: %v", err)
	}
	// check that the stdout buffer contains the expected output
	match = strings.Contains(stdout.String(), "0.000000")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())
	// provide two filenames that will be compared
	stdout, stderr, err = grok(emptyStdin, "similarity", "testdata/sim1.md", "testdata/sim2.md")
	Tassert(t, err == nil, "CLI returned unexpected error: %v %v", err, stderr.String())
	// check that the stdout buffer contains the expected output
	match = strings.Contains(stdout.String(), "0.875")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	///////////////////////////////////////////////////////////////
	// everything below here requires a grokker repository to be
	// initialized and takes place in that repository
	///////////////////////////////////////////////////////////////

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

	// test TokenCount
	stdinTokenCount := bytes.Buffer{}
	stdinTokenCount.WriteString("token count test")
	stdout, stderr, err = grok(stdinTokenCount, "tc")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = cimatch(stdout.String(), "3")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// initialize a grokker repository
	stdout, stderr, err = grok(emptyStdin, "init")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

	// try initializing a grokker repository in a directory that already has one
	stdout, stderr, err = grok(emptyStdin, "init")
	Tassert(t, err != nil, "CLI returned no error when it should have")

	// pass in a command line that should work
	stdout, stderr, err = grok(emptyStdin, "models")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = strings.Contains(stdout.String(), "gpt-")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())
	// check that the stderr buffer is empty
	Tassert(t, stderr.String() == "", "CLI returned unexpected error: %s", stderr.String())

	// set model
	stdout, stderr, err = grok(emptyStdin, "model", "gpt-4")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

	// test msg
	msgStdin := bytes.Buffer{}
	msgStdin.WriteString("1 == 2")
	stdout, stderr, err = grok(msgStdin, "msg", "you are a logic machine.  answer the provided question.  say answer=true or answer=false.")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = strings.Contains(stdout.String(), "answer=false")

	// create a file
	f, err := os.Create("test.txt")
	Tassert(t, err == nil, "error creating file: %v", err)
	// put some content in the file
	_, err = f.WriteString("testing is good")
	Tassert(t, err == nil, "error writing to file: %v", err)
	// close the file
	err = f.Close()

	// add the file to the repository
	stdout, stderr, err = grok(emptyStdin, "add", "test.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

	// ask a question
	stdout, stderr, err = grok(emptyStdin, "q", "Does the context claim testing is good?  Answer yes or no.")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = cimatch(stdout.String(), "yes") || cimatch(stdout.String(), "Yes")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// try adding the file again
	stdout, stderr, err = grok(emptyStdin, "add", "test.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer is empty
	Tassert(t, stdout.String() == "", "CLI returned unexpected output: %s", stdout.String())
	// check that the stderr buffer is empty
	Tassert(t, stderr.String() == "", "CLI returned unexpected error: %s", stderr.String())

	// try adding a file that doesn't exist
	stdout, stderr, err = grok(emptyStdin, "add", "test2.txt")
	// if the file doesn't exist, err will be an *fs.PathError
	Tassert(t, err != nil, "CLI returned no error when it should have")
	Tassert(t, err.(*fs.PathError).Err == syscall.ENOENT, "CLI returned unexpected error: %#v", err)

	// create and add a couple of files we'll forget
	mkFile(t, "test2.txt", "forget daisies")
	mkFile(t, "test3.txt", "forget submarines")
	stdout, stderr, err = grok(emptyStdin, "add", "test2.txt", "test3.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// make sure the files are in the repository
	stdout, stderr, err = grok(emptyStdin, "q", "what should we forget?")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	match = cimatch(stdout.String(), "daisies") && cimatch(stdout.String(), "submarines")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// forget the files using relative and absolute paths
	stdout, stderr, err = grok(emptyStdin, "forget", "test2.txt", Spf("%s/test3.txt", dir))
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// make sure the files are no longer in the repository
	stdout, stderr, err = grok(emptyStdin, "q", "what should we forget?")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	match = !cimatch(stdout.String(), "daisies") && !cimatch(stdout.String(), "submarines")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// test continuation
	mkFile(t, "continue.txt", "roses are red, violets are blue, grokker is great, and so are you!")
	stdout, stderr, err = grok(emptyStdin, "add", "continue.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// create a stdin buffer with the preface
	preface := "roses are red,"
	stdinContinue := bytes.Buffer{}
	stdinContinue.WriteString(preface)
	// run the CLI with the preface as stdin
	stdout, stderr, err = grok(stdinContinue, "qc")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// fmt.Println(stdout.String())
	match = cimatch(stdout.String(), "violets are blue")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// test context
	fmt.Println("testing context...")
	stdinContext := bytes.Buffer{}
	stdinContext.WriteString("roses are red")
	stdout, stderr, err = grok(stdinContext, "ctx", "30")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = strings.Contains(stdout.String(), "violets are blue")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// test revision
	stdinRevision := bytes.Buffer{}
	stdinRevision.WriteString("roses are orange")
	// run the CLI with the incorrect text as stdin
	stdout, stderr, err = grok(stdinRevision, "qr")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// fmt.Println(stdout.String())
	match = cimatch(stdout.String(), "are red")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// test revision with custom sysmsg
	stdinRevisionWithSysmsg := bytes.Buffer{}
	stdinRevisionWithSysmsg.WriteString("you are an expert botanist.  make corrections in the color of roses based on the provided context.\n\nroses are orange")
	// run the CLI with the incorrect text as stdin
	stdout, stderr, err = grok(stdinRevisionWithSysmsg, "qr", "-s")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// fmt.Println(stdout.String())
	match = cimatch(stdout.String(), "red")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// test revision with custom sysmsg but one paragraph
	stdinRevisionWithSysmsg = bytes.Buffer{}
	stdinRevisionWithSysmsg.WriteString("this is a one-paragraph input that should not work.")
	// run the CLI with the incorrect text as stdin
	stdout, stderr, err = grok(stdinRevisionWithSysmsg, "qr", "-s")
	Tassert(t, err != nil, "CLI returned no error when it should have")

	// test backup
	stdout, stderr, err = grok(emptyStdin, "backup")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

	// test UpdateEmbeddings
	// create and add a file we'll add, change, then unlink
	mkFile(t, "deleteme.txt", "this file will not be deleted")
	stdout, stderr, err = grok(emptyStdin, "add", "deleteme.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// add the file to the repository
	stdout, stderr, err = grok(emptyStdin, "add", "deleteme.txt")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check the file's contents
	stdout, stderr, err = grok(emptyStdin, "q", "will a file be deleted? Say answer=yes or answer=no.")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = cimatch(stdout.String(), "answer=no")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())
	// change the file's contents
	f, err = os.OpenFile("deleteme.txt", os.O_WRONLY, 0644)
	Tassert(t, err == nil, "error opening file: %v", err)
	_, err = f.WriteString("this file will be deleted")
	Tassert(t, err == nil, "error writing to file: %v", err)
	err = f.Close()
	Tassert(t, err == nil, "error closing file: %v", err)
	// check the file's contents
	stdout, stderr, err = grok(emptyStdin, "q", "will a file be deleted? Say answer=yes or answer=no.")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = cimatch(stdout.String(), "answer=yes")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())
	// delete the file
	err = os.Remove("deleteme.txt")
	Tassert(t, err == nil, "error deleting file: %v", err)
	// check the file's contents
	stdout, stderr, err = grok(emptyStdin, "q", "is one of the files named deleteme.txt? Say answer=yes or answer=no.")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = cimatch(stdout.String(), "answer=no")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// test DBVersion
	stdout, stderr, err = grok(emptyStdin, "version")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = strings.Contains(stdout.String(), Spf("db version %s", core.Version))

	// test refresh
	// get list of files in the db
	stdout, stderr, err = grok(emptyStdin, "ls")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer mentions deleteme.txt
	match = cimatch(stdout.String(), "deleteme.txt")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())
	// do the refresh
	stdout, stderr, err = grok(emptyStdin, "refresh")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	stdout, stderr, err = grok(emptyStdin, "ls")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer does not mention deleteme.txt
	match = !strings.Contains(stdout.String(), "deleteme.txt")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// test embed subcommand
	// provide a phrase that will be embedded
	stdinEmbed := bytes.Buffer{}
	stdinEmbed.WriteString("roses are red")
	stdout, stderr, err = grok(stdinEmbed, "embed")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = strings.Contains(stdout.String(), "-0.001")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// test git commit
	// git init
	run(t, "git", "init")
	// git add
	run(t, "git", "add", ".")
	// git diff
	stdout, stderr, err = grok(emptyStdin, "commit")
	Tassert(t, err == nil, "CLI returned unexpected error: %v\nstdout: %v\nstderr: %v", err, stdout.String(), stderr.String())
	// check that the stdout buffer contains the expected output
	match = strings.Contains(stdout.String(), "diff --git")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())
	match = strings.Contains(stdout.String(), "test.txt")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())
	match = strings.Contains(stdout.String(), "add")

	// test locking
	fmt.Println("testing locking...")
	// use a channel to track lock/unlock sequence
	seq := make(chan int, 99)
	// start a goroutine that will lock the db
	go func() {
		seq <- 0
		// lock the db by calling LoadFrom directly
		_, _, _, _, lock, err := core.LoadFrom(".grok", "", false)
		Tassert(t, err == nil, "error locking db: %v", err)
		seq <- 1
		// sleep for a bit; this should block the other goroutine
		time.Sleep(3 * time.Second)
		// unlock the db
		err = lock.Unlock()
		Tassert(t, err == nil, "error unlocking db: %v", err)
		seq <- 3
	}()
	// start another goroutine that will lock the db
	go func() {
		// sleep for a bit to let the other goroutine lock the db
		time.Sleep(1 * time.Second)
		seq <- 2
		// try to lock the db; this should block until the other goroutine unlocks it
		_, _, _, _, lock, err := core.LoadFrom(".grok", "", false)
		Tassert(t, err == nil, "error locking db: %v", err)
		seq <- 4
		// unlock the db
		err = lock.Unlock()
		Tassert(t, err == nil, "error unlocking db: %v", err)
		seq <- 5
		close(seq)
	}()
	// check the sequence
	var i int
	for got := range seq {
		Tassert(t, got == i, "expected %d, got %d", i, got)
		i++
	}

	// test read-only locking
	// ensure msg does not lock the db
	// start a goroutine that will lock the db
	fmt.Println("testing non-locking...")
	seq = make(chan int, 99)
	go func() {
		seq <- 0
		// lock the db by calling LoadFrom directly
		_, _, _, _, lock, err := core.LoadFrom(".grok", "", true)
		Tassert(t, err == nil, "error locking db: %v", err)
		seq <- 1
		// sleep for a bit; this should not block msg
		time.Sleep(3 * time.Second)
		seq <- 4
		// unlock the db
		err = lock.Unlock()
		Tassert(t, err == nil, "error unlocking db: %v", err)
		seq <- 5
		close(seq)
	}()
	// start another goroutine that will run msg
	go func() {
		// sleep for a bit to let the other goroutine lock the db
		time.Sleep(1 * time.Second)
		seq <- 2
		// try to run msg; this should not block
		lockStdin := bytes.Buffer{}
		lockStdin.WriteString("1 == 2")
		stdout, stderr, err = grok(lockStdin, "msg", "you will answer the provided question with one word.")
		Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
		seq <- 3
	}()
	// check the sequence
	i = 0
	for got := range seq {
		Tassert(t, got == i, "expected %d, got %d", i, got)
		i++
	}

	// try to cause locking race conditions for a few seconds
	fmt.Println("testing for locking race conditions...")
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// try running a command that will modify the db
			// - we use 'model' because it modifies the db but doesn't
			//   contact any servers
			// - if we get db data corruption then locking failed
			stdout, stderr, err = grok(emptyStdin, "model", "gpt-4")
			Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
		}()
	}
	wg.Wait()

}

func TestCliChat(t *testing.T) {

	var stdout, stderr bytes.Buffer
	_ = stderr
	var emptyStdin bytes.Buffer
	var match bool
	var err error

	///////////////////////////////////////////////////////////////
	// everything below here requires a grokker repository to be
	// initialized and takes place in that repository
	///////////////////////////////////////////////////////////////

	// get current working directory
	cwd, err := os.Getwd()
	Tassert(t, err == nil, "error getting current working directory: %v", err)
	// create a temporary directory
	dir, err := os.MkdirTemp("", "grokker")
	Ck(err)
	Pf("dir: %s\n", dir)
	// defer os.RemoveAll(dir)
	// cd into the temporary directory
	cd(t, dir)
	defer cd(t, cwd)

	// initialize a grokker repository
	stdout, stderr, err = grok(emptyStdin, "init")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

	// set model
	stdout, stderr, err = grok(emptyStdin, "model", "gpt-4")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)

	// test chat with no files in the db
	msgStdin := bytes.Buffer{}
	msgStdin.WriteString("Let's discuss neural nets.")
	stdout, stderr, err = grok(msgStdin, "chat", "nets")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = cimatch(stdout.String(), "neural")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())
	msgStdin.Reset()
	msgStdin.WriteString("What were we talking about?")
	stdout, stderr, err = grok(msgStdin, "chat", "nets")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = cimatch(stdout.String(), "neural")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// see if the nets file got indexed
	stdout, stderr, err = grok(emptyStdin, "ls")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = cimatch(stdout.String(), "nets")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// make sure the nets file is showing up in a different chat context
	msgStdin.Reset()
	msgStdin.WriteString("What were we talking about?")
	stdout, stderr, err = grok(msgStdin, "chat", "roses", "-C")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the stdout buffer contains the expected output
	match = cimatch(stdout.String(), "neural")
	Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	// test sysmsg flag and output file
	msgStdin.Reset()
	msgStdin.WriteString("Write a simple neural net in Go.")
	stdout, stderr, err = grok(msgStdin, "chat", "nets",
		"-s", "You are a neural net expert and Go developer.",
		"-o", "net.go")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// check that the output file exists
	if _, err := os.Stat("net.go"); os.IsNotExist(err) {
		Tassert(t, false, "output file does not exist")
	}
	// get the contents of the output file
	bufA, err := os.ReadFile("net.go")
	Tassert(t, err == nil, "error reading file: %v", err)

	// generate a different output file
	msgStdin.Reset()
	msgStdin.WriteString("Write a simple neural net in Go with two layers.")
	stdout, stderr, err = grok(msgStdin, "chat", "nets",
		"-o", "net.go")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// get the contents of the output file
	bufB, err := os.ReadFile("net.go")
	Tassert(t, err == nil, "error reading file: %v", err)
	// check that the output file changed
	Tassert(t, !bytes.Equal(bufA, bufB), "output file did not change")

	// extract the original output file using -x 2 -O, which should not change the file on disk
	stdout, stderr, err = grok(emptyStdin, "chat", "nets",
		"-x", "2", "-O",
		"-o", "net.go")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// get the contents of the output file
	bufC, err := os.ReadFile("net.go")
	Tassert(t, err == nil, "error reading file: %v", err)
	// check that the output file did not change
	Tassert(t, bytes.Equal(bufB, bufC), dumpDiff(bufB, bufC))

	// extract the original output file using -x 2, which should make
	// the file on disk identical to the bufA version
	stdout, stderr, err = grok(emptyStdin, "chat", "nets",
		"-x", "2",
		"-o", "net.go")
	Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
	// get the contents of the output file
	bufD, err := os.ReadFile("net.go")
	Tassert(t, err == nil, "error reading file: %v", err)
	// check that the output file is the same as the original output file
	Tassert(t, bytes.Equal(bufA, bufD), dumpDiff(bufA, bufD))

}
