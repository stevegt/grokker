package main

import (
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	oai "github.com/sashabaranov/go-openai"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker"
)

// parse args using kong package
var cli struct {
	Init struct{} `cmd:"" help:"Initialize a new .grok file in the current directory."`
	Add  struct {
		Paths []string `arg:"" type:"existingfile" help:"Path to file to add to knowledge base."`
	} `cmd:"" help:"Add a file to the knowledge base."`
	Refresh struct{} `cmd:"" help:"Refresh the embeddings for all documents in the knowledge base."`
	Ls      struct{} `cmd:"" help:"List all documents in the knowledge base."`
	Q       struct {
		Question string `arg:"" help:"Question to ask the knowledge base."`
	} `cmd:"" help:"Ask the knowledge base a question."`
	Qi      struct{} `cmd:"" help:"Ask the knowledge base a question on stdin."`
	Commit  struct{} `cmd:"" help:"Generate a git commit message on stdout."`
	Models  struct{} `cmd:"" help:"List all available models."`
	Upgrade struct {
		Model string `arg:"" help:"Model to upgrade to."`
	} `cmd:"" help:"Upgrade the model used by the knowledge base."`
	Global  bool `short:"g" help:"Include results from OpenAI's global knowledge base as well as from local documents."`
	Verbose bool `short:"v" help:"Show debug and progress information on stderr."`
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
		// XXX use the default model for now, but we should accept an
		// optional model name as an init argument
		grok, err := grokker.New("")
		Ck(err)
		// save it to .grok
		fh, err := os.Create(".grok")
		Ck(err)
		err = grok.Save(fh)
		Ck(err)
		fh.Close()
		Pl("Initialized a new .grok file in the current directory.")
		os.Exit(0)
	}

	// find the .grok file in the current or any parent directory
	grokfnbase := ".grok"
	grokpath := ""
	for level := 0; level < 10; level++ {
		path := strings.Repeat("../", level) + grokfnbase
		if _, err := os.Stat(path); err == nil {
			grokpath = path
			break
		}
	}
	if grokpath == "" {
		Fpf(os.Stderr, "No .grok file found in current directory or any parent directory.\n")
		os.Exit(1)
	}

	// get the last modified time of the .grok file
	fi, err := os.Stat(grokpath)
	Ck(err)
	timestamp := fi.ModTime()

	// load the .grok file
	fh, err := os.Open(grokpath)
	grok, err := grokker.Load(fh)
	Ck(err)
	fh.Close()

	save := false
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
	case "refresh":
		// refresh the embeddings for all documents
		err = grok.RefreshEmbeddings()
		Ck(err)
		// save the .grok file
		save = true
	case "ls":
		// list the documents in the .grok file
		for _, doc := range grok.Documents {
			Pl(doc.Path)
		}
	case "q <question>":
		// get question from args and print the answer
		if cli.Q.Question == "" {
			Fpf(os.Stderr, "Error: q command requires a question argument\n")
			os.Exit(1)
		}
		question := cli.Q.Question
		resp, _, updated, err := answer(grok, timestamp, question, cli.Global)
		Ck(err)
		Pl(resp.Choices[0].Message.Content)
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
		resp, query, updated, err := answer(grok, timestamp, question, cli.Global)
		Ck(err)
		_ = query
		Pf("\n%s\n\n%s\n\n", question, resp.Choices[0].Message.Content)
		if updated {
			save = true
		}
	case "commit":
		// generate a git commit message
		resp, err := commitMessage(grok)
		Ck(err)
		Pf("%s", resp.Choices[0].Message.Content)
	case "models":
		// list all available models
		models, err := grok.ListModels()
		Ck(err)
		for _, model := range models {
			Pl(model)
		}
	case "upgrade <model>":
		// upgrade the model used by the knowledge base
		err := grok.UpgradeModel(cli.Upgrade.Model)
		Ck(err)
		Pf("Upgraded model to %s\n", cli.Upgrade.Model)
		save = true
	default:
		Fpf(os.Stderr, "Error: unrecognized command: %s\n", ctx.Command())
		os.Exit(1)
	}

	if save {
		// save the grok file
		Debug("saving grok file")
		tmpfn := grokpath + ".tmp"
		fh, err := os.Create(tmpfn)
		err = grok.Save(fh)
		Ck(err)
		fh.Close()
		err = os.Rename(tmpfn, grokpath)
		Ck(err)
		Debug(" done!")
	}

}

// answer a question
func answer(grok *grokker.Grokker, timestamp time.Time, question string, global bool) (resp oai.ChatCompletionResponse, query string, updated bool, err error) {
	defer Return(&err)

	// update the knowledge base
	updated, err = grok.UpdateEmbeddings(timestamp)
	Ck(err)

	// answer the question
	resp, query, err = grok.Answer(question, global)
	Ck(err)

	return
}

// generate a git commit message
func commitMessage(grok *grokker.Grokker) (resp oai.ChatCompletionResponse, err error) {
	defer Return(&err)

	// run `git diff --staged`
	cmd := exec.Command("git", "diff", "--staged")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	Ck(err)
	diff := string(out)

	// call grokker
	resp, _, err = grok.GitCommitMessage(diff)
	Ck(err)

	return
}
