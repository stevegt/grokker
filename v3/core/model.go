package core

import (
	"fmt"
	"sort"

	oai "github.com/sashabaranov/go-openai"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/client"
)

var DefaultModel = "o3-mini"

// Model is a type for model name and characteristics
type Model struct {
	Name         string
	TokenLimit   int
	providerName string
	upstreamName string
	active       bool
	provider     client.ChatClient
}

func (m *Model) String() string {
	status := ""
	if m.active {
		status = "*"
	}
	return fmt.Sprintf("%1s %-20s %-20s tokens: %d)", status, m.Name, m.providerName, m.TokenLimit)
}

// GetModel returns the current model name and model_t from the db
func (g *Grokker) GetModel() (model string, m *Model, err error) {
	defer Return(&err)
	model, m, err = g.models.FindModel(g.Model)
	Ck(err)
	return
}

// Models is a type that manages the set of available models.
type Models struct {
	// The list of available models.
	Available map[string]*Model
}

// NewModels creates a new Models object.
func NewModels() (models *Models) {
	models = &Models{}
	models.Available = make(map[string]*Model)
	add := func(name string, tokenLimit int, providerName string, upstreamName string) {
		m := &Model{
			Name:         name,
			TokenLimit:   tokenLimit,
			providerName: providerName,
			upstreamName: upstreamName,
		}
		models.Available[name] = m
	}

	add("gpt-3.5-turbo", 4096, "openai", oai.GPT3Dot5Turbo)
	add("gpt-4", 8192, "openai", oai.GPT4)
	add("gpt-4-32k", 32768, "openai", oai.GPT432K)
	add("gpt-4-turbo-preview", 128000, "openai", oai.GPT4TurboPreview)
	add("gpt-4o", 128000, "openai", oai.GPT4o)
	add("o1-preview", 128000, "openai", oai.O1Preview)
	add("o1-mini", 128000, "openai", oai.O1Mini)
	add("o1", 128000, "openai", oai.O1Preview)
	add("o3", 200000, "openai", oai.O3)
	add("o3-mini", 200000, "openai", oai.O3Mini)

	// XXX perplexity input token limits are not published?
	add("sonar-deep-research", 128000, "perplexity", "sonar-deep-research")
	add("sonar", 128000, "perplexity", "sonar")
	add("sonar-pro", 128000, "perplexity", "sonar-pro")
	add("sonar-reasoning", 128000, "perplexity", "sonar-reasoning")
	add("sonar-reasoning-pro", 128000, "perplexity", "sonar-reasoning-pro")
	add("r1-1776", 128000, "perplexity", "r1-1776")

	return
}

// FindModel returns the model name and object given a model name.
// if the given model name is empty, then use DefaultModel.
func (models *Models) FindModel(model string) (name string, m *Model, err error) {
	if model == "" {
		model = DefaultModel
	}
	m, ok := models.Available[model]
	if !ok {
		err = fmt.Errorf("model %q not found", model)
		return
	}
	name = model
	return
}

// ListModels returns a list of available models sorted by provider
// name and model name.
func (models *Models) ListModels() (list []*Model) {
	for _, m := range models.Available {
		list = append(list, m)
	}
	// sort by provider name and model name
	sort.Slice(list, func(i, j int) bool {
		if list[i].providerName == list[j].providerName {
			return list[i].Name < list[j].Name
		}
		return list[i].providerName < list[j].providerName
	})
	return
}

// Setup the model and oai clients.
// This function needs to be idempotent because it might be called multiple
// times during the lifetime of a Grokker object.
func (g *Grokker) Setup(model string) (err error) {
	defer Return(&err)
	g.initEmbeddingClient()
	err = g.initModel(model)
	Ck(err)
	err = InitTokenizer()
	Ck(err)
	return
}

// initModel initializes the model for a new or reloaded Grokker database.
// This function needs to be idempotent because it might be called multiple
// times during the lifetime of a Grokker object.
//
// XXX rename this to initDefaultModel, and always pass model name to
// LLM API calls. This will make it easier to use multiple models in
// the same run.
func (g *Grokker) initModel(model string) (err error) {
	defer Return(&err)
	Assert(g.Root != "", "root directory not set")
	g.models = NewModels()
	model, m, err := g.models.FindModel(model)
	Ck(err)
	m.active = true
	// XXX make Model be the most recently used model name
	g.Model = model
	g.ModelObj = m
	// XXX EmbeddingTokenLimit hardcoded for the text-embedding-ada-002 model
	g.EmbeddingTokenLimit = 8192
	return
}
