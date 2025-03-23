package cli

import (
	"flag"
	"os"
	"testing"

	"github.com/stevegt/grokker/v3/core"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/util"
)

var modelName = "gpt-4"

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
	teFullBuf, err := os.ReadFile("../core/testdata/te-full.txt")
	Tassert(t, err == nil, "error reading file: %v", err)
	largeChatBuf, err := os.ReadFile("../core/testdata/large-chat")
	Tassert(t, err == nil, "error reading file: %v", err)

	//get current directory
	prevDir, err := os.Getwd()
	Tassert(t, err == nil, "error getting current directory: %v", err)
	defer os.Chdir(prevDir)

	// cd to the tmp dir
	dir := core.TmpTestDir()
	err = os.Chdir(dir)
	Tassert(t, err == nil, "error changing directory: %v", err)

	// create a new Grokker database
	grok, err := core.Init(dir, "gpt-4")
	Tassert(t, err == nil, "error creating grokker: %v", err)
	// initialize Tokenizer
	err = core.InitTokenizer()
	Ck(err)
	defer grok.Save()

	// start a chat by mentioning something not in GPT-4's global context
	res, err := grok.Chat(modelName, "", "Pretend a blue widget has a red center.", "chat1", util.ContextAll, nil, nil, 0, 0, false, true, false)
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
	res, debug, err := history.ContinueChat(modelName, "Talk about complex systems.", util.ContextAll, nil, nil, 0, false)
	Tassert(t, err == nil, "error continuing chat: %v", err)
	err = history.Save(true)
	Tassert(t, err == nil, "error saving chat history: %v", err)
	// should take no more than a few iterations to reach half the token limit
	tokenLimit := grok.ModelObj.TokenLimit
	ok := false
	for i := 0; i < 3; i++ {
		Pf("iteration %d\n", i)
		ctx, err := grok.Context("system", 3000, false, false)
		Tassert(t, err == nil, "error getting context: %v", err)
		prompt := Spf("%s\n\nDiscuss complex systems more.", ctx)
		res, debug, err = history.ContinueChat(modelName, prompt, util.ContextAll, nil, nil, 0, false)
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
	res, err = grok.Chat(modelName, "", "What color is the center of the blue widget?", "chat1", util.ContextAll, nil, nil, 0, 0, false, true, false)
	match = cimatch(res, "red")
	if !match {
		Pf("dir: %s\n", dir)
		Pf("modelName: %s\n", modelName)
		Pf("res: %s\n", res)
		t.Fatal("CLI did not return expected output")
	}

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
			res, _, err = history.ContinueChat(modelName, prompt, util.ContextAll, nil, nil, 0, false)
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
	// res, err = grok.Chat(modelName, "", "What color is the center of the blue widget?", "chat1", util.ContextAll, nil, nil, 0, 0, false, true, false)
	res, err = grok.Chat(modelName, "", "From the context, what widget has a red center?", "chat1", util.ContextAll, nil, nil, 0, 0, false, true, false)
	match = cimatch(res, "red")
	if !match {
		Pf("dir: %s\n", dir)
		Pf("modelName: %s\n", modelName)
		Pf("res: %s\n", res)
		t.Fatal("CLI did not return expected output")
	}

}
