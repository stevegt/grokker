module ga-evolve

go 1.22.1

replace github.com/sashabaranov/go-openai => /home/stevegt/lab/go-openai

replace github.com/stevegt/grokker/v3 => /home/stevegt/lab/grokker/v3

require (
	github.com/stevegt/goadapt v0.7.0
	github.com/stevegt/grokker/v3 v3.0.0-00010101000000-000000000000
)

require (
	github.com/dlclark/regexp2 v1.9.0 // indirect
	github.com/fabiustech/openai v0.4.0 // indirect
	github.com/gofrs/flock v0.8.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/sashabaranov/go-openai v1.29.2 // indirect
	github.com/stevegt/envi v0.2.0 // indirect
	github.com/stevegt/semver v0.0.0-20240217000820-5913d1a31c26 // indirect
	github.com/tiktoken-go/tokenizer v0.1.0 // indirect
	golang.org/x/sys v0.27.0 // indirect
)
