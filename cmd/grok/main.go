package main

import (
	"os"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker"
	// "github.com/joho/godotenv"
)

func main() {
	docfn := os.Args[1]
	question := os.Args[2]

	// get the current directory
	dir, err := os.Getwd()
	Ck(err)
	// see if there's a .grok file in the current directory
	grokfn := dir + "/.grok"
	var grok *grokker.Grokker
	if _, err := os.Stat(grokfn); err != nil {
		Pf("No .grok file found in current directory -- creating one...")
		grok = grokker.New()
		// add the document
		err = grok.AddDocument(docfn)
		Ck(err)
		// save the grok file
		fh, err := os.Create(grokfn)
		err = grok.Save(fh)
		Ck(err)
		Pl("done!")
	} else {
		// load the .grok file
		fh, err := os.Open(grokfn)
		grok, err = grokker.Load(fh)
		Ck(err)
	}

	// answer the question
	resp, query, err := grok.Answer(question)
	Ck(err)
	_ = query
	Pprint(resp)
}
