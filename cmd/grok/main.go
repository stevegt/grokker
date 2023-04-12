package main

import (
	"io/ioutil"
	"os"
	"strings"

	oai "github.com/sashabaranov/go-openai"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker"

	// "github.com/joho/godotenv"
	// "github.com/docopt/docopt-go"
	"github.com/namsral/flag"
)

var usage = `
Usage:
	grok add <docfn>
	grok [-g] q <question> 

Options:
	-g  Include results from OpenAI's global knowledge base as well as
		from local documents.
`

func main() {
	// parse args using flag package
	global := flag.Bool("g", false, "Include results from OpenAI's global knowledge base as well as from local documents.")
	flag.Parse()
	args := flag.Args()
	cmd := args[0]

	// get the current directory
	dir, err := os.Getwd()
	Ck(err)
	grokfn := dir + "/.grok"

	var grok *grokker.Grokker
	switch cmd {
	case "add":
		if len(args) < 2 {
			Pl("Error: add command requires a filename argument")
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
		}

		// add the documents
		for _, docfn := range args[1:] {
			// add the document
			Pf(" adding %s...", docfn)
			err = grok.AddDocument(docfn)
			Ck(err)
		}
		// save the grok file
		fh, err := os.Create(grokfn)
		err = grok.Save(fh)
		Ck(err)
		Pl(" done!")
	case "q":
		// get question from args and print the answer
		if len(args) < 2 {
			Pl("Error: q command requires a question argument")
			os.Exit(1)
		}
		question := args[len(args)-1]
		resp, query, err := answer(grokfn, question, *global)
		Ck(err)
		_ = query
		// Pprint(resp)
		Pl(resp.Choices[0].Message.Content)
	case "qi":
		// get question from stdin and print both question and answer
		buf, err := ioutil.ReadAll(os.Stdin)
		Ck(err)
		question := string(buf)
		// trim whitespace
		question = strings.TrimSpace(question)
		resp, query, err := answer(grokfn, question, *global)
		Ck(err)
		_ = query
		Pf("\n%s\n\n%s\n\n", question, resp.Choices[0].Message.Content)
	default:
		Pl("Error: unrecognized command")
		os.Exit(1)
	}
}

// answer a question
func answer(grokfn, question string, global bool) (resp oai.ChatCompletionResponse, query string, err error) {
	defer Return(&err)

	// see if there's a .grok file in the current directory
	// XXX we should probably do this in the caller
	if _, err := os.Stat(grokfn); err != nil {
		Pl("No .grok file found in current directory.")
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
