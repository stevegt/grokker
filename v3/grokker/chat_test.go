package grokker

import (
	"flag"
	"os"
	"testing"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/util"
)

/*
in Go, is there a way of making a test case not run unless a flag is set?

Yes, you can achieve this in Go by using conditional logic in your test and command-line flags provided by the `flag` package. You declare a flag at the package level, and then in your test function, you use an if statement to check if the flag is set. If the flag is not set, you can call `t.Skip()` to skip the test:

```go
package mypackage

import (
	"flag"
	"testing"
)

var runMyTest = flag.Bool("runMyTest", false, "run this specific test")

func TestMyFunction(t *testing.T) {
	if !*runMyTest {
		t.Skip("skipping test; to run, use: -runMyTest=true")
	}
    // Your test code here
}
```

When running your tests, you can decide to run this test case with the `-runMyTest=true` flag:

```bash
go test -runMyTest=true
```
*/

var runChatSummarization = flag.Bool("runChatSummarization", false, "run chat summarization test")

// test summarization in a large chat
func TestChatSummarization(t *testing.T) {
	if !*runChatSummarization {
		t.Skip("skipping test; to run, use: -runChatSummarization=true")
	}

	var match bool

	// get some test data before changing directories
	teFullBuf, err := os.ReadFile("testdata/te-full.txt")
	Tassert(t, err == nil, "error reading file: %v", err)
	largeChatBuf, err := os.ReadFile("testdata/large-chat")
	Tassert(t, err == nil, "error reading file: %v", err)

	dir := tmpDir()
	// cd to the tmp dir
	err = os.Chdir(dir)
	Tassert(t, err == nil, "error changing directory: %v", err)

	// create a new Grokker database
	grok, err := Init(dir, "gpt-4")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// initialize Tokenizer
	err = InitTokenizer()
	Ck(err)
	defer grok.Save()

	// start a chat by mentioning something not in GPT-4's global context
	res, err := grok.Chat("", "Pretend a blue widget has a red center.", "chat1", util.ContextAll, nil, nil, 0, 0, false, true)
	Tassert(t, err == nil, "error starting chat: %v", err)
	// check that the response contains the expected output
	match = cimatch(res, "red")
	Tassert(t, match, "CLI did not return expected output: %s", res)

	// copy te-full.txt to the current directory
	err = os.WriteFile("te-full.txt", teFullBuf, 0644)
	Tassert(t, err == nil, "error writing file: %v", err)
	// add the file to the database so we get a large context in
	// subsequent queries
	err = grok.AddDocument("te-full.txt")
	Tassert(t, err == nil, "error adding file: %v", err)

	// generate a lot of context to cause summarization
	Pl("testing large context")
	history, err := grok.OpenChatHistory("", "chat1")
	Tassert(t, err == nil, "error opening chat history: %v", err)
	res, debug, err := history.continueChat("Talk about complex systems.", util.ContextAll, nil, nil, 0)
	Tassert(t, err == nil, "error continuing chat: %v", err)
	err = history.Save(true)
	Tassert(t, err == nil, "error saving chat history: %v", err)
	// should take no more than a few iterations to reach half the token limit
	tokenLimit := grok.tokenLimit
	ok := false
	for i := 0; i < 3; i++ {
		Pf("iteration %d\n", i)
		ctx, err := grok.Context("system", 3000, false, false)
		Tassert(t, err == nil, "error getting context: %v", err)
		prompt := Spf("%s\n\nDiscuss complex systems more.", ctx)
		res, debug, err = history.continueChat(prompt, util.ContextAll, nil, nil, 0)
		Tassert(t, err == nil, "error continuing chat: %v", err)
		err = history.Save(true)
		Ck(err)
		// ensure the response contains the expected output
		match = cimatch(res, "system")
		Tassert(t, match, "CLI did not return expected output: %s", res)
		// ensure final token count is less than half the token limit
		Tassert(t, debug["finalCount"] < tokenLimit/2, "final token count is too high: %v", debug)
		// check peak token count
		Pf("final token count: %d\n", debug["finalCount"])
		Pf("peak token count: %d\n", debug["peakCount"])
		if debug["peakCount"] > tokenLimit/2 {
			ok = true
			break
		}
	}
	Tassert(t, ok, "peak token count never exceeded token limit: %v", debug)

	// check that we still remember the blue widget
	res, err = grok.Chat("", "What color is the center of the blue widget?", "chat1", util.ContextAll, nil, nil, 0, 0, false, true)
	match = cimatch(res, "red")
	Tassert(t, match, "CLI did not return expected output: %s", res)

	// now grow the chat file itself to be larger than the token limit
	Pl("testing large chat file")
	ok = false
	for i := 0; i < 10; i++ {
		if true {
			// use a prebaked chat file to save testing time
			Pl("using prebaked chat file")
			err = os.WriteFile("chat1", largeChatBuf, 0644)
			Tassert(t, err == nil, "error writing file: %v", err)
			// add the file to the database
			err = grok.AddDocument("chat1")
			Tassert(t, err == nil, "error adding file: %v", err)
		} else {
			Pl("generating chat file")
			ctx, err := grok.Context("system", 3000, false, false)
			Tassert(t, err == nil, "error getting context: %v", err)
			prompt := Spf("Discuss this topic more:\n%s\n\n", ctx)
			res, _, err = history.continueChat(prompt, util.ContextAll, nil, nil, 0)
			Tassert(t, err == nil, "error continuing chat: %v", err)
			err = history.Save(true)
			Ck(err)
			// ensure the response contains the expected output
			match = cimatch(res, "system")
			Tassert(t, match, "CLI did not return expected output: %s", res)
		}
		// check chat1 file token count
		buf, err := os.ReadFile("chat1")
		Tassert(t, err == nil, "error reading file: %v", err)
		count, err := grok.TokenCount(string(buf))
		Tassert(t, err == nil, "error counting tokens: %v", err)
		if count > tokenLimit {
			ok = true
			break
		}
	}
	Tassert(t, ok, "chat1 file never exceeded token limit: %v", debug)

	// check that we still remember the blue widget
	res, err = grok.Chat("", "What color is the center of the blue widget?", "chat1", util.ContextAll, nil, nil, 0, 0, false, true)
	match = cimatch(res, "red")
	Tassert(t, match, "CLI did not return expected output: %s", res)

}
