package core

import (
	"context"
	"os"
	"time"

	embedLib "github.com/fabiustech/openai"
	embedModelLib "github.com/fabiustech/openai/models"
	. "github.com/stevegt/goadapt"
)

// XXX using only OpenAI for embedding -- need to support more providers

// initEmbeddingClient initializes the OpenAI embedding client.
func (g *Grokker) initEmbeddingClient() {
	authtoken := os.Getenv("OPENAI_API_KEY")
	g.embeddingClient = embedLib.NewClient(authtoken)
}

// createEmbeddings returns the embeddings for a slice of text chunks.
func (g *Grokker) createEmbeddings(texts []string) (embeddings [][]float64, err error) {
	defer Return(&err)
	// use github.com/fabiustech/openai library
	c := g.embeddingClient
	// simply call c.CreateEmbeddings() once for each text chunk.
	for i := 0; i < len(texts); i++ {
		text := texts[i]
		// set empty chunk embedding to nil
		if len(text) == 0 {
			embeddings = append(embeddings, nil)
			continue
		}
		inputs := []string{text}
		req := &embedLib.EmbeddingRequest{
			Input: inputs,
			Model: embedModelLib.AdaEmbeddingV2,
		}
		Debug("creating embedding for chunk %d of %d ...", i+1, len(texts))
		// Debug("text: %q", text)
		// loop with backoff until we get a response
		var res *embedLib.EmbeddingResponse
		for backoff := 1; backoff < 10; backoff++ {
			res, err = c.CreateEmbeddings(context.Background(), req)
			if err == nil {
				break
			}
			Pf("openai API error, retrying: %#v", err)
			// wait and try again
			time.Sleep(time.Second * time.Duration(backoff))
		}
		Ck(err, "%T: %#v", err, err)
		for _, em := range res.Data {
			embeddings = append(embeddings, em.Embedding)
		}
	}
	Debug("created %d embeddings", len(embeddings))
	Assert(len(embeddings) <= len(texts))
	return
}
