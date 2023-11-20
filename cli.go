package grokker

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/alecthomas/kong"
	. "github.com/stevegt/goadapt"
)

// XXX move to cli/cli.go

/*

XXX Migrate all of this to grokker.go, including kong; just call
grokker.Cli(os.Args, os.Stdin, os.Stdout, os.Stderr) from main().
argv to cli.go Cli() in the grokker package.  That will give us the most
flexibility later when importing into PUP, WASM, CC, etc. That will
also shift the API surface to be the same as the cli subcommands,
and will also make it easier to test the cli subcommands.

is there a kong function that can parse a non-default args array?

Yes, Kong provides a function called `Parse(args []string)` that can
parse a non-default args array. This function takes an array of
strings (args) and parses the command line arguments from that array
instead of the default `os.Args`.

show me

I'm sorry for the confusion, but the Kong library in Go does not directly provide a function to parse a non-default args array. The Kong library's `Parse()` function does not take any argument as an input for an arguments array. It directly uses the `os.Args` slice which is populated with command-line arguments.

However, Go's `os.Args` slice could be directly manipulated before calling `kong.Parse()` to achieve similar functionality. Such as:

```go
os.Args = append(os.Args[:1], args...)
ctx := kong.Parse(&cli)
```

In the above code, `args` is your custom arguments array slice. Here, the slice `os.Args[1:]` (i.e., the original command-line arguments excluding the program name itself) is replaced with your custom arguments array, `args`.

*/

// parse args using kong package
var cli struct {
	Add struct {
		Paths []string `arg:"" type:"string" help:"Path to file to add to knowledge base."`
	} `cmd:"" help:"Add a file to the knowledge base."`
	Backup struct{} `cmd:"" help:"Backup the knowledge base."`
	Commit struct{} `cmd:"" help:"Generate a git commit message on stdout."`
	Forget struct {
		Paths []string `arg:"" type:"string" help:"Path to file to remove from knowledge base."`
	} `cmd:"" help:"Forget about a file, removing it from the knowledge base."`
	Global bool     `short:"g" help:"Include results from OpenAI's global knowledge base as well as from local documents."`
	Init   struct{} `cmd:"" help:"Initialize a new .grok file in the current directory."`
	Ls     struct{} `cmd:"" help:"List all documents in the knowledge base."`
	Models struct{} `cmd:"" help:"List all available models."`
	Model  struct {
		Model string `arg:"" help:"Model to switch to."`
	} `cmd:"" help:"Upgrade the model used by the knowledge base."`
	Msg struct {
		Sysmsg string `arg:"" help:"System message to send to control behavior of openAI's API."`
	} `cmd:"" help:"Send message to openAI's API from stdin and print response on stdout."`
	Q struct {
		Question string `arg:"" help:"Question to ask the knowledge base."`
	} `cmd:"" help:"Ask the knowledge base a question."`
	Qc      struct{} `cmd:"" help:"Continue text from stdin based on the context in the knowledge base."`
	Qi      struct{} `cmd:"" help:"Ask the knowledge base a question on stdin."`
	Qr      struct{} `cmd:"" help:"Revise stdin based on the context in the knowledge base."`
	Refresh struct{} `cmd:"" help:"Refresh the embeddings for all documents in the knowledge base."`
	SysMsg  bool     `short:"s" help:"expect sysmsg in first paragraph of stdin, return same on stdout."`
	Tc      struct{} `cmd:"" help:"Calculate the token count of stdin."`
	Verbose bool     `short:"v" help:"Show debug and progress information on stderr."`
	Version struct{} `cmd:"" help:"Show version of grok and its database."`
}

// Config contains the configuration for grokker
type Config struct {
	// Name is the name of the program
	Name string
	// Description is a short description of the program
	Description string
	// Version is the version of the program
	Version string
	// Exit is the function to call to exit the program
	Exit   func(int)
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// NewConfig returns a new Config struct with default values populated
func NewConfig() *Config {
	return &Config{
		Name:        "grokker",
		Description: "A command-line tool for having a conversation with a set of local documents and the OpenAI API.",
		Version:     CodeVersion(),
		Exit:        func(i int) { os.Exit(i) },
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	}
}

// Cli parses the given arguments and then executes the appropriate
// subcommand.
//
// We use this function instead of kong.Parse() so that we can pass in
// the arguments to parse.  This allows us to more easily test the
// cli subcommands, and could later ease e.g. WASM usage.
func Cli(args []string, config *Config) (rc int, err error) {
	defer Return(&err)

	options := []kong.Option{
		kong.Name(config.Name),
		kong.Description(config.Description),
		kong.Exit(config.Exit),
		kong.Writers(config.Stdout, config.Stderr),
		kong.Vars{
			"version": config.Version,
		},
	}

	var parser *kong.Kong
	parser, err = kong.New(&cli, options...)
	Ck(err)
	ctx, err := parser.Parse(args)
	parser.FatalIfErrorf(err)

	Debug("ctx: %+v", ctx)

	if cli.Verbose {
		os.Setenv("DEBUG", "1")
	}

	cmd := ctx.Command()

	// list of commands that don't require an existing database
	noDbCmds := []string{"init", "msg", "tc"}
	needsDb := true
	if stringInSlice(cmd, noDbCmds) {
		needsDb = false
	}

	var grok *GrokkerInternal
	var migrated, save bool
	var was, now string
	// initialize Tokenizer
	err = InitTokenizer()
	Ck(err)
	// initialize Grokker object if needed
	if needsDb {
		grok, migrated, was, now, err = Load()
		Ck(err)
		if migrated {
			// save the old db
			var fn string
			fn, err = grok.Backup()
			Ck(err)
			Pf("migrated grokker db from version %s to %s\n", was, now)
			Pf("backup of old db saved to %s\n", fn)
			save = true
		}
	}

	// XXX replace this with "command pattern" or "command object"
	switch cmd {
	case "init":
		// initialize a new .grok file in the current directory
		// create a new Grokker object
		// XXX assume current directory for now, but should be able to
		// specify rootdir on command line
		// XXX use the default model for now, but we should accept an
		// optional model name as an init argument
		_, err = Init(".", "")
		Ck(err)
		Pl("Initialized a new .grok file in the current directory.")
		// Init calls Save() for us
		return
	case "add <paths>":
		if len(cli.Add.Paths) < 1 {
			Fpf(config.Stderr, "Error: add command requires a filename argument\n")
			rc = 1
			return
		}
		// add the documents
		for _, docfn := range cli.Add.Paths {
			// add the document
			Fpf(os.Stderr, " adding %s...\n", docfn)
			err = grok.AddDocument(docfn)
			if err != nil {
				return
			}
		}
		// save the grok file
		save = true
	case "forget <paths>":
		if len(cli.Forget.Paths) < 1 {
			Fpf(config.Stderr, "Error: forget command requires a filename argument\n")
			rc = 1
			return
		}
		// forget the documents
		for _, docfn := range cli.Forget.Paths {
			// forget the document
			Fpf(os.Stderr, " forgetting %s...\n", docfn)
			err = grok.ForgetDocument(docfn)
			Ck(err)
		}
		// save the grok file
		save = true
	case "refresh":
		// refresh the embeddings for all documents
		err = grok.RefreshEmbeddings()
		Ck(err)
		// save the db
		save = true
	case "ls":
		// list the documents in the knowledge base
		paths := grok.ListDocuments()
		for _, path := range paths {
			Pl(path)
		}
	case "q <question>":
		// get question from args and print the answer
		if cli.Q.Question == "" {
			Fpf(config.Stderr, "Error: q command requires a question argument\n")
			rc = 1
			return
		}
		question := cli.Q.Question
		resp, _, updated, err := answer(grok, question, cli.Global)
		Ck(err)
		Pl(resp)
		if updated {
			save = true
		}
	case "qc":
		// get text from stdin and print both text and continuation
		buf, err := ioutil.ReadAll(config.Stdin)
		Ck(err)
		txt := string(buf)
		// trim whitespace
		// txt = strings.TrimSpace(txt)
		resp, _, updated, err := cont(grok, txt, cli.Global)
		Ck(err)
		Pf("%s\n%s\n", txt, resp)
		if updated {
			save = true
		}
	case "qi":
		// get question from stdin and print both question and answer
		buf, err := ioutil.ReadAll(config.Stdin)
		Ck(err)
		question := string(buf)
		// trim whitespace
		question = strings.TrimSpace(question)
		resp, query, updated, err := answer(grok, question, cli.Global)
		Ck(err)
		_ = query
		Pf("\n%s\n\n%s\n\n", question, resp)
		if updated {
			save = true
		}
	case "qr":
		// get content from stdin and emit revised version on stdout
		buf, err := ioutil.ReadAll(config.Stdin)
		Ck(err)
		in := string(buf)
		// in = strings.TrimSpace(in)
		out, updated, err := revise(grok, in, cli.Global, cli.SysMsg)
		Ck(err)
		// Pf("%s\n\n%s\n", sysmsg, out)
		// Pf("%s\n\n%s\n\n", in, out)
		Pf("%s", out)
		if updated {
			save = true
		}
	case "tc":
		// get content from stdin and emit token count on stdout
		buf, err := ioutil.ReadAll(config.Stdin)
		Ck(err)
		in := string(buf)
		in = strings.TrimSpace(in)
		count, err := grok.TokenCount(in)
		Ck(err)
		Pf("%d\n", count)
	case "msg <sysmsg>":
		// get message from stdin and print response
		buf, err := ioutil.ReadAll(config.Stdin)
		Ck(err)
		input := string(buf)
		// trim whitespace
		input = strings.TrimSpace(input)
		sysmsg := cli.Msg.Sysmsg
		res, err := msg(grok, sysmsg, input)
		Ck(err)
		Pl(res)
	case "commit":
		// generate a git commit message
		summary, err := commitMessage(grok)
		Ck(err)
		Pl(summary)
	case "models":
		// list all available models
		models, err := grok.ListModels()
		Ck(err)
		for _, model := range models {
			Pl(model)
		}
	case "model <model>":
		// upgrade the model used by the knowledge base
		oldModel, err := grok.SetModel(cli.Model.Model)
		Ck(err)
		Pf("Switched model from %s to %s\n", oldModel, cli.Model.Model)
		save = true
	case "version":
		// print the version of grokker
		Pf("grokker version %s\n", CodeVersion())
		// print the version of the grok db
		Pf("grok db version %s\n", grok.DBVersion())
	case "backup":
		// backup the grok db
		fn, err := grok.Backup()
		Ck(err)
		Pf("backup of grok db saved to %s\n", fn)
	default:
		Fpf(config.Stderr, "Error: unrecognized command: %s\n", ctx.Command())
		rc = 1
		return
	}

	if save {
		// save the grok file
		err = grok.Save()
		Ck(err)
	}

	return
}

// answer a question
func answer(grok *GrokkerInternal, question string, global bool) (resp, query string, updated bool, err error) {
	defer Return(&err)

	// update the knowledge base
	updated, err = grok.UpdateEmbeddings()
	Ck(err)

	// answer the question
	resp, err = grok.Answer(question, global)
	Ck(err)

	return
}

// continue text
func cont(grok *GrokkerInternal, in string, global bool) (resp, query string, updated bool, err error) {
	defer Return(&err)

	// update the knowledge base
	updated, err = grok.UpdateEmbeddings()
	Ck(err)

	// continue the text
	Debug("cont: in: %s", in)
	resp, query, err = grok.Continue(in, global)
	Ck(err)

	return
}

// revise text
func revise(grok *GrokkerInternal, in string, global, sysmsgin bool) (out string, updated bool, err error) {
	defer Return(&err)

	// update the knowledge base
	updated, err = grok.UpdateEmbeddings()
	Ck(err)

	// return revised text
	out, _, err = grok.Revise(in, global, sysmsgin)
	Ck(err)

	return
}

// send a message to openAI's API
func msg(g *GrokkerInternal, sysmsg string, input string) (res string, err error) {
	defer Return(&err)
	respmsg, err := g.msg(sysmsg, input)
	Ck(err)
	res = respmsg.Choices[0].Message.Content
	return
}

// generate a git commit message
func commitMessage(grok *GrokkerInternal) (summary string, err error) {
	defer Return(&err)

	// run `git diff --staged`
	cmd := exec.Command("git", "diff", "--staged")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	Ck(err)
	diff := string(out)

	// call grokker
	summary, err = grok.GitCommitMessage(diff)
	Ck(err)

	return
}
