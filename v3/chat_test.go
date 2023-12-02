package grokker

import (
	"os"
	"testing"

	. "github.com/stevegt/goadapt"
)

// test summarization in a large chat
func TestChatSummarization(t *testing.T) {

	var match bool

	dir := tmpDir()
	// cd to the tmp dir
	err := os.Chdir(dir)
	Tassert(t, err == nil, "error changing directory: %v", err)

	// create a new Grokker database
	grok, err := Init(dir, "gpt-4")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// initialize Tokenizer
	err = InitTokenizer()
	Ck(err)

	// start a chat
	res, err := grok.Chat("", "Tell me everything you know about jets.", "jets")
	Tassert(t, err == nil, "error starting chat: %v", err)
	// check that the response contains the expected output
	match = cimatch(res, "jet")
	Tassert(t, match, "CLI did not return expected output: %s", res)

	// continue the chat a bunch of times
	for i := 0; i < 10; i++ {
		prompt := Spf("Write a long story about a jet.")
		res, err = grok.Chat("", prompt, "jets")
		Tassert(t, err == nil, "error continuing chat: %v", err)
	}

	/*

		// generate a large chat by just feeding the output back in
		msgStdin.Reset()
		msgStdin.WriteString("Tell me everything you know about roses.")
		stdout, stderr, err = grok(msgStdin, "chat", "roses")
		Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
		// feed the output back in a bunch of times
		for i := 0; i < 10; i++ {
			msgStdin.Reset()
			msgStdin.WriteString(stdout.String())
			stdout, stderr, err = grok(msgStdin, "chat", "roses")
			Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
		}
		// check that the file is at least double the token limit
		txt, err := os.ReadFile("roses")
		Tassert(t, err == nil, "error reading file: %v", err)
		// run tc on the file
		msgStdin.Reset()
		msgStdin.Write(txt)
		stdout, stderr, err = grok(msgStdin, "tc")
		Tassert(t, err == nil, "CLI returned unexpected error: %v", err)
		// convert the output to an int
		tc, err := strconv.Atoi(strings.TrimSpace(stdout.String()))
		Tassert(t, err == nil, "error converting token count to int: %v", err)
		tokenLimit := 8192 // XXX get from model
		Tassert(t, tc > 2*tokenLimit, "file is not large enough: %d", tc)
		// check that the stdout buffer contains the expected output
		match = cimatch(stdout.String(), "roses")
		Tassert(t, match, "CLI did not return expected output: %s", stdout.String())
		// see if there's still some neural net content in the file
		match = cimatch(stdout.String(), "neural")
		Tassert(t, match, "CLI did not return expected output: %s", stdout.String())

	*/
}
