package main

import (
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/alecthomas/kong"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker"
)

// XXX migrate all of this to grokker.go, including kong; just pass
// argv to grokker.go.  that will give us the most flexibility later
// when importing into PUP, WASM, CC, etc.

// parse args using kong package
var cli struct {
	Init struct{} `cmd:"" help:"Initialize a new .grok file in the current directory."`
	Add  struct {
		Paths []string `arg:"" type:"string" help:"Path to file to add to knowledge base."`
	} `cmd:"" help:"Add a file to the knowledge base."`
	Forget struct {
		Paths []string `arg:"" type:"string" help:"Path to file to remove from knowledge base."`
	} `cmd:"" help:"Forget about a file, removing it from the knowledge base."`
	Refresh struct{} `cmd:"" help:"Refresh the embeddings for all documents in the knowledge base."`
	Ls      struct{} `cmd:"" help:"List all documents in the knowledge base."`
	Q       struct {
		Question string `arg:"" help:"Question to ask the knowledge base."`
	} `cmd:"" help:"Ask the knowledge base a question."`
	Qc  struct{} `cmd:"" help:"Continue text from stdin based on the context in the knowledge base."`
	Qi  struct{} `cmd:"" help:"Ask the knowledge base a question on stdin."`
	Qr  struct{} `cmd:"" help:"Revise stdin based on the context in the knowledge base."`
	Tc  struct{} `cmd:"" help:"Calculate the token count of stdin."`
	Msg struct {
		Sysmsg string `arg:"" help:"System message to send to control behavior of openAI's API."`
	} `cmd:"" help:"Send message to openAI's API from stdin and print response on stdout."`
	Global  bool     `short:"g" help:"Include results from OpenAI's global knowledge base as well as from local documents."`
	SysMsg  bool     `short:"s" help:"expect sysmsg in first paragraph of stdin, return same on stdout."`
	Verbose bool     `short:"v" help:"Show debug and progress information on stderr."`
	Version struct{} `cmd:"" help:"Show version of grok and its database."`
	Commit  struct{} `cmd:"" help:"Generate a git commit message on stdout."`
	Models  struct{} `cmd:"" help:"List all available models."`
	Model   struct {
		Model string `arg:"" help:"Model to switch to."`
	} `cmd:"" help:"Upgrade the model used by the knowledge base."`
}

func main() {
	ctx := kong.Parse(&cli,
		kong.Name("grok"),
		kong.Description("A command-line tool for having a conversation with a set of documents."),
	)

	Debug("ctx: %+v", ctx)

	if cli.Verbose {
		os.Setenv("DEBUG", "1")
	}

	if ctx.Command() == "init" {
		// initialize a new .grok file in the current directory
		// create a new Grokker object
		// XXX assume current directory for now, but should be able to
		// specify rootdir on command line
		// XXX use the default model for now, but we should accept an
		// optional model name as an init argument
		_, err := grokker.Init(".", "")
		Ck(err)
		Pl("Initialized a new .grok file in the current directory.")
		os.Exit(0)
	}

	save := false
	grok, migrated, was, now, err := grokker.Load()
	Ck(err)
	if migrated {
		fn, err := grok.Backup()
		Ck(err)
		Pf("migrated grokker db from version %s to %s\n", was, now)
		Pf("backup of old db saved to %s\n", fn)
		save = true
	}

	// XXX move all of this to a sub
	switch ctx.Command() {
	case "add <paths>":
		if len(cli.Add.Paths) < 1 {
			Fpf(os.Stderr, "Error: add command requires a filename argument\n")
			os.Exit(1)
		}
		// add the documents
		for _, docfn := range cli.Add.Paths {
			// add the document
			Fpf(os.Stderr, " adding %s...\n", docfn)
			err = grok.AddDocument(docfn)
			Ck(err)
		}
		// save the grok file
		save = true
	case "forget <paths>":
		if len(cli.Forget.Paths) < 1 {
			Fpf(os.Stderr, "Error: forget command requires a filename argument\n")
			os.Exit(1)
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
			Fpf(os.Stderr, "Error: q command requires a question argument\n")
			os.Exit(1)
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
		buf, err := ioutil.ReadAll(os.Stdin)
		Ck(err)
		txt := string(buf)
		// trim whitespace
		txt = strings.TrimSpace(txt)
		resp, _, updated, err := answer(grok, txt, cli.Global)
		Ck(err)
		Pf("%s\n%s\n", txt, resp)
		if updated {
			save = true
		}
	case "qi":
		// get question from stdin and print both question and answer
		buf, err := ioutil.ReadAll(os.Stdin)
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
		buf, err := ioutil.ReadAll(os.Stdin)
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
		buf, err := ioutil.ReadAll(os.Stdin)
		Ck(err)
		in := string(buf)
		in = strings.TrimSpace(in)
		count, err := grok.TokenCount(in)
		Ck(err)
		Pf("%d\n", count)
	case "msg <sysmsg>":
		// get message from stdin and print response
		buf, err := ioutil.ReadAll(os.Stdin)
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
		Pf("grokker version %s\n", grok.CodeVersion())
		// print the version of the grok db
		Pf("grok db version %s\n", grok.DBVersion())
	default:
		Fpf(os.Stderr, "Error: unrecognized command: %s\n", ctx.Command())
		os.Exit(1)
	}

	if save {
		// save the grok file
		err = grok.Save()
		Ck(err)
	}

}

// answer a question
func answer(grok *grokker.Grokker, question string, global bool) (resp, query string, updated bool, err error) {
	defer Return(&err)

	// update the knowledge base
	updated, err = grok.UpdateEmbeddings()
	Ck(err)

	// answer the question
	resp, err = grok.Answer(question, global)
	Ck(err)

	return
}

// revise text
func revise(grok *grokker.Grokker, in string, global, sysmsgin bool) (out string, updated bool, err error) {
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
func msg(grok *grokker.Grokker, sysmsg string, input string) (res string, err error) {
	defer Return(&err)

	// return response
	res, err = grok.Msg(sysmsg, input)
	Ck(err)

	return
}

// generate a git commit message
func commitMessage(grok *grokker.Grokker) (summary string, err error) {
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
