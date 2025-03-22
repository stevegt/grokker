package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
)

type Request struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Response struct {
	Citations []Citation `json:"citations"`
	Choices   []Choice   `json:"choices"`
	Error     *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type Citation string

type Choice struct {
	FinishReason string  `json:"finish_reason"`
	Role         string  `json:"role"`
	Message      Message `json:"message"`
}

func main() {

	url := "https://api.perplexity.ai/chat/completions"

	// build the request
	Request := Request{
		Model: "sonar-deep-research",
		Messages: []Message{
			{
				Role:    "system",
				Content: "Be precise and concise.",
			},
			{
				Role:    "user",
				Content: "How many stars are there in our galaxy?",
			},
		},
	}

	payload, err := json.Marshal(Request)
	if err != nil {
		panic(err)
	}

	fmt.Println(string(payload))

	// create reader from string
	payloadReader := strings.NewReader(string(payload))

	req, _ := http.NewRequest("POST", url, payloadReader)

	token := os.Getenv("PERPLEXITY_API_KEY")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Add("Content-Type", "application/json")

	res, _ := http.DefaultClient.Do(req)

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	fmt.Println(string(body))

	// parse the response using json.Unmarshal and the Response struct
	var response Response
	err = json.Unmarshal(body, &response)
	if err != nil {
		panic(err)
	}
	if response.Error != nil {
		fmt.Println("Error:", response.Error.Message)
		return
	}

	fmt.Println("Choices:")
	for i, choice := range response.Choices {
		fmt.Printf("[%d] %s:\n    %s\n", i, choice.Role, choice.Message.Content)
	}

	fmt.Println("Citations:")
	for i, citation := range response.Citations {
		fmt.Printf("[%d] %s\n", i+1, citation)
	}

}
