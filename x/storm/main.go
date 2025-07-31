package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"
)

var tmpl = template.Must(template.New("index").Parse(`
<html>
<head>
  <meta charset="utf-8">
  <title>Grokker LLM Chat</title>
  <style>
    body { font-family: Arial, sans-serif; margin: 0; padding: 0; }
    #chat { padding: 20px; max-height: 80vh; overflow-y: auto; border-bottom: 1px solid #ccc; }
    .message { margin-bottom: 10px; padding: 5px; border: 1px solid #ddd; border-radius: 4px; background-color: #f9f9f9; }
    #input-area { position: fixed; bottom: 0; width: 100%; background: #f0f0f0; padding: 10px; box-shadow: 0 -2px 5px rgba(0,0,0,0.1); }
    textarea { width: 70%; height: 50px; vertical-align: middle; margin-right: 10px; }
    select { vertical-align: middle; margin-right: 10px; }
    button { height: 54px; vertical-align: middle; }
  </style>
</head>
<body>
  <div id="chat">
    <!-- Chat messages will appear here -->
  </div>
  <div id="input-area">
    <select id="llmSelect">
      <option value="LLM1">LLM1</option>
      <option value="LLM2">LLM2</option>
    </select>
    <textarea id="userInput" placeholder="Enter your query or comment"></textarea>
    <button id="sendBtn">Send</button>
  </div>
  <script>
    // Append a new message to the chat view.
    function appendMessage(content) {
      var chat = document.getElementById("chat");
      var messageDiv = document.createElement("div");
      messageDiv.className = "message";
      messageDiv.innerHTML = content;
      // Enable selection-based commenting.
      messageDiv.addEventListener("mouseup", function(e) {
        var selection = window.getSelection().toString().trim();
        if(selection.length > 0) {
          if(confirm("Would you like to comment on: " + selection + "?")) {
            var comment = prompt("Enter your comment:");
            if(comment) {
              // When commenting, send the comment along with the context (selected text).
              sendQuery(comment, document.getElementById("llmSelect").value, selection);
            }
          }
        }
      });
      chat.appendChild(messageDiv);
      chat.scrollTop = chat.scrollHeight;
    }

    // Send query or comment to the /query endpoint.
    function sendQuery(query, llm, context) {
      fetch("/query", {
        method: "POST",
        headers: {
          "Content-Type": "application/json"
        },
        body: JSON.stringify({ query: query, llm: llm, context: context })
      }).then(function(response) {
        return response.json();
      }).then(function(data) {
        appendMessage(data.response);
      }).catch(function(err) {
        appendMessage("Error: " + err);
      });
    }

    // Handle click on the Send button.
    document.getElementById("sendBtn").addEventListener("click", function() {
      var input = document.getElementById("userInput");
      var query = input.value;
      if(query.trim() === "") return;
      sendQuery(query, document.getElementById("llmSelect").value, "");
      input.value = "";
    });
  </script>
</body>
</html>
`))

// QueryRequest represents a user's query or comment input.
type QueryRequest struct {
	Query   string `json:"query"`
	LLM     string `json:"llm"`
	Context string `json:"context"`
}

// QueryResponse represents the LLM's response.
type QueryResponse struct {
	Response string `json:"response"`
}

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := tmpl.Execute(w, nil); err != nil {
			http.Error(w, "Template error", http.StatusInternalServerError)
		}
	})

	http.HandleFunc("/query", queryHandler)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting server on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

// queryHandler processes each query or comment, sends it to the Grokker API (simulated),
// and returns the LLM response as HTML.
func queryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Call the simulated Grokker API LLM service.
	responseText := sendQueryToLLM(req.Query, req.LLM, req.Context)

	resp := QueryResponse{
		Response: markdownToHTML(responseText),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// sendQueryToLLM simulates calling the Grokker API to obtain a markdown-formatted text.
// In a production system, this would make an HTTP request to the Grokker API.
func sendQueryToLLM(query string, llm string, context string) string {
	// Simulate processing delay.
	time.Sleep(1 * time.Second)
	if context != "" {
		return fmt.Sprintf("Response for comment on '%s': %s [via %s]", context, query, llm)
	}
	return fmt.Sprintf("Response from %s: %s", llm, query)
}

// markdownToHTML converts markdown text to HTML.
// For simplicity, this function wraps the escaped markdown in a paragraph.
// A production system might use a full-featured markdown parser.
func markdownToHTML(markdown string) string {
	return "<p>" + template.HTMLEscapeString(markdown) + "</p>"
}
