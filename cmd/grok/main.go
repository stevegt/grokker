package main

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/alecthomas/kong"
	oai "github.com/sashabaranov/go-openai"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker"
)

// parse args using kong package
var cli struct {
	Add struct {
		Paths []string `arg:"" type:"existingfile" help:"Path to file to add to knowledge base."`
	} `cmd:"" help:"Add a file to the knowledge base."`
	Refresh struct{} `cmd:"" help:"Refresh the embeddings for all documents in the knowledge base."`
	Ls      struct{} `cmd:"" help:"List all documents in the knowledge base."`
	Q       struct {
		Question string `arg:"" help:"Question to ask the knowledge base."`
	} `cmd:"" help:"Ask the knowledge base a question."`
	Qi      struct{} `cmd:"" help:"Ask the knowledge base a question on stdin."`
	Global  bool     `short:"g" help:"Include results from OpenAI's global knowledge base as well as from local documents."`
	Verbose bool     `short:"v" help:"Show debug and progress information on stderr."`
}

func main() {
	ctx := kong.Parse(&cli,
		kong.Name("grok"),
		kong.Description("A command-line tool for having a conversation with a set of documents."),
	)

	if cli.Verbose {
		os.Setenv("DEBUG", "1")
	}

	// get the current directory
	dir, err := os.Getwd()
	Ck(err)
	grokfn := dir + "/.grok"

	var grok *grokker.Grokker
	switch ctx.Command() {
	case "add":
		if len(cli.Add.Paths) < 1 {
			Fpf(os.Stderr, "Error: add command requires a filename argument")
			os.Exit(1)
		}
		// create a new .grok file if it doesn't exist
		if _, err := os.Stat(grokfn); os.IsNotExist(err) {
			grok = grokker.New()
		} else {
			// load the .grok file
			fh, err := os.Open(grokfn)
			grok, err = grokker.Load(fh)
			Ck(err)
			fh.Close()
		}

		// add the documents
		for _, docfn := range cli.Add.Paths {
			// add the document
			Debug(" adding %s...", docfn)
			err = grok.AddDocument(docfn)
			Ck(err)
		}
		// save the grok file
		fh, err := os.Create(grokfn)
		err = grok.Save(fh)
		Ck(err)
		fh.Close()
		Debug(" done!")
	case "refresh":
		// refresh the embeddings for all documents
		// load the .grok file
		fh, err := os.Open(grokfn)
		grok, err = grokker.Load(fh)
		Ck(err)
		fh.Close()
		// refresh the embeddings
		err = grok.RefreshEmbeddings()
		Ck(err)
		// save the .grok file
		fh, err = os.Create(grokfn)
		err = grok.Save(fh)
		Ck(err)
		fh.Close()
	case "ls":
		// list the documents in the .grok file
		fh, err := os.Open(grokfn)
		Ck(err)
		grok, err := grokker.Load(fh)
		Ck(err)
		for _, doc := range grok.Documents {
			Pl(doc.Path)
		}
	case "q":
		// get question from args and print the answer
		if cli.Q.Question == "" {
			Fpf(os.Stderr, "Error: q command requires a question argument")
			os.Exit(1)
		}
		question := cli.Q.Question
		resp, _, err := answer(grokfn, question, cli.Global)
		Ck(err)
		Pl(resp.Choices[0].Message.Content)
	case "qi":
		// get question from stdin and print both question and answer
		buf, err := ioutil.ReadAll(os.Stdin)
		Ck(err)
		question := string(buf)
		// trim whitespace
		question = strings.TrimSpace(question)
		resp, query, err := answer(grokfn, question, cli.Global)
		Ck(err)
		_ = query
		Pf("\n%s\n\n%s\n\n", question, resp.Choices[0].Message.Content)
	default:
		Fpf(os.Stderr, "Error: unrecognized command")
		os.Exit(1)
	}
}

// answer a question
func answer(grokfn, question string, global bool) (resp oai.ChatCompletionResponse, query string, err error) {
	defer Return(&err)

	// see if there's a .grok file in the current directory
	// XXX we should probably do this in the caller
	if _, err := os.Stat(grokfn); err != nil {
		Fpf(os.Stderr, "No .grok file found in current directory.")
		os.Exit(1)
	}

	// load the .grok file
	// XXX we should probably do this in the caller
	fh, err := os.Open(grokfn)
	grok, err := grokker.Load(fh)
	Ck(err)
	fh.Close()

	// update the knowledge base
	update, err := grok.UpdateEmbeddings(grokfn)
	Ck(err)

	// answer the question
	resp, query, err = grok.Answer(question, global)
	Ck(err)

	// save the .grok file if it was updated
	// XXX we should probably do this in the caller
	if update {
		fh, err := os.Create(grokfn)
		Ck(err)
		err = grok.Save(fh)
		Ck(err)
		fh.Close()
	}
	return
}
