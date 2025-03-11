package core

import (
	"fmt"

	oai "github.com/sashabaranov/go-openai"
	. "github.com/stevegt/goadapt"
)

var DefaultModel = "o3-mini"

// Model is a type for model name and characteristics
type Model struct {
	Name         string
	TokenLimit   int
	providerName string
	upstreamName string
	active       bool
	provider     ChatProvider
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
	add("o3-mini", 200000, "openai", oai.O3Mini)
	add("sonar-deep-research", 128000, "perplexity", "sonar-deep-research")

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

// Setup the model and oai clients.
// This function needs to be idempotent because it might be called multiple
// times during the lifetime of a Grokker object.
func (g *Grokker) Setup(model string) (err error) {
	defer Return(&err)
	err = g.initModel(model)
	Ck(err)
	g.initClients()
	err = InitTokenizer()
	Ck(err)
	return
}

// initModel initializes the model for a new or reloaded Grokker database.
// This function needs to be idempotent because it might be called multiple
// times during the lifetime of a Grokker object.
func (g *Grokker) initModel(model string) (err error) {
	defer Return(&err)
	Assert(g.Root != "", "root directory not set")
	g.models = NewModels()
	model, m, err := g.models.FindModel(model)
	Ck(err)
	m.active = true
	g.Model = model
	g.modelObj = m
	// XXX replace with a real tokenizer.
	// charsPerToken := 3.1
	// g.maxChunkLen = int(math.Floor(float64(m.TokenLimit) * charsPerToken))
	// XXX replace with a real tokenizer.
	// g.maxEmbeddingChunkLen = int(math.Floor(float64(8192) * charsPerToken))
	g.TokenLimit = m.TokenLimit
	//TokenLimithardcoded for the text-embedding-ada-002 model
	g.EmbeddingTokenLimit = 8192
	return
}
