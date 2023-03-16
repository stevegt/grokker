package main

import (
	"os"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker"
	// "github.com/joho/godotenv"
)

func main() {
	cmd := os.Args[1]
	docfn := os.Args[2]

	var question string

	// get the current directory
	dir, err := os.Getwd()
	Ck(err)
	grokfn := dir + "/.grok"

	var grok *grokker.Grokker
	switch cmd {
	case "add":
		Pf("Creating .grok file...")
		grok = grokker.New()
		// add the document
		err = grok.AddDocument(docfn)
		Ck(err)
		// save the grok file
		fh, err := os.Create(grokfn)
		err = grok.Save(fh)
		Ck(err)
		Pl("done!")
	case "q":
		question = os.Args[3]
		// see if there's a .grok file in the current directory
		if _, err := os.Stat(grokfn); err != nil {
			Pl("No .grok file found in current directory.")
			os.Exit(1)
		}
		// load the .grok file
		fh, err := os.Open(grokfn)
		grok, err = grokker.Load(fh)
		Ck(err)

		// answer the question
		resp, query, err := grok.Answer(question)
		Ck(err)
		_ = query
		// Pprint(resp)
		Pl(resp.Choices[0].Message.Content)
	default:
		Pl("Unknown command:", cmd)
		os.Exit(1)
	}
}
