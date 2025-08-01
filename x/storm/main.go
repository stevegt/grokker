package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/stevegt/grokker/v3/client"
	"github.com/stevegt/grokker/v3/core"
	"github.com/yuin/goldmark"
)

var tmpl = template.Must(template.New("index").Parse(`
<html>
<head>
  <meta charset="utf-8">
  <title>Grokker LLM Chat</title>
  <style>
    body { font-family: Arial, sans-serif; margin: 0; padding: 0; }
    #chat { padding: 20px; max-height: 80vh; overflow-y: auto; border-bottom: 1px solid #ccc; overflow-anchor: none; }
    .message { margin-bottom: 10px; padding: 5px; border: 1px solid #ddd; border-radius: 4px; background-color: #f9f9f9; }
    #spinner-area { padding: 10px; text-align: center; }
    .spinner {
      border: 4px solid #f3f3f3;
      border-top: 4px solid #3498db;
      border-radius: 50%;
      width: 10px;
      height: 10px;
      animation: spin 1s linear infinite;
      display: inline-block;
      margin-right: 5px;
    }
    @keyframes spin {
      0% { transform: rotate(0deg); }
      100% { transform: rotate(360deg); }
    }
    #input-area { position: fixed; bottom: 0; width: 100%; background: #f0f0f0; padding: 10px; box-shadow: 0 -2px 5px rgba(0,0,0,0.1); }
    textarea { width: 70%; height: 50px; vertical-align: middle; margin-right: 10px; }
    select { vertical-align: middle; margin-right: 10px; }
    button { height: 54px; vertical-align: middle; }
  </style>
</head>
<body>
  <div id="chat">
    <!-- Chat messages will appear here -->
    {{.ChatHTML}}
  </div>
  <div id="spinner-area">
    <!-- Progress spinners will appear here -->
  </div>
  <div id="input-area">
    <select id="llmSelect">
      <option value="o3-mini">o3-mini</option>
      <option value="sonar-deep-research">sonar-deep-research</option>
    </select>
    <textarea id="userInput" placeholder="Enter query"></textarea>
    <button id="sendBtn">Send</button>
  </div>
  <script>
    // Append a new message to the chat view without scrolling the page.
    function appendMessage(content) {
      var chat = document.getElementById("chat");
      var messageDiv = document.createElement("div");
      messageDiv.className = "message";
      messageDiv.innerHTML = content;
      chat.appendChild(messageDiv);
    }

    // Send query to the /query endpoint.
    // Each query is immediately added to the chat with a 10px spinner.
    // When the LLM response is received the spinner is removed and replaced by the response.
    function sendQuery(query, llm, context) {
      var chat = document.getElementById("chat");
      var messageDiv = document.createElement("div");
      messageDiv.className = "message";
	  messageDiv.innerHTML = "<strong>" + query + "</strong>";
      // Create a spinner element next to the query.
      var spinner = document.createElement("span");
      spinner.className = "spinner";
      spinner.style.marginLeft = "10px";
      messageDiv.appendChild(spinner);
      chat.appendChild(messageDiv);

      fetch("/query", {
        method: "POST",
        headers: {
          "Content-Type": "application/json"
        },
        body: JSON.stringify({ query: query, llm: llm, context: context })
      }).then(function(response) {
        return response.json();
      }).then(function(data) {
        // Remove the spinner once the response is received.
        spinner.remove();
        var responseDiv = document.createElement("div");
        responseDiv.innerHTML = data.response;
        messageDiv.appendChild(responseDiv);
      }).catch(function(err) {
        spinner.remove();
        var errorDiv = document.createElement("div");
        errorDiv.textContent = "Error: " + err;
        messageDiv.appendChild(errorDiv);
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

    // Enable selection-based querying on the chat messages.
    document.addEventListener("mouseup", function(e) {
      console.log("Mouse up event detected");
      var selection = window.getSelection().toString().trim();
      if(selection.length > 0) {
        console.log("Selected text: " + selection);
        var query = prompt("Enter your query:");
        if(!query) {
			query = "Expand on: " + selection;
		}
	    sendQuery(query, document.getElementById("llmSelect").value, selection);
      }
      return;
    });
  </script>
</body>
</html>
`))

// QueryRequest represents a user's query input.
type QueryRequest struct {
	Query   string `json:"query"`
	LLM     string `json:"llm"`
	Context string `json:"context"`
}

// QueryResponse represents the LLM's response.
type QueryResponse struct {
	Response string `json:"response"`
}

var markdownFilename string
var chatHistory string
var chatMutex sync.Mutex

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	filePtr := flag.String("file", "", "markdown file to store chat history")
	flag.Parse()
	if *filePtr == "" {
		log.Fatal("must provide a markdown filename with -file")
	}
	markdownFilename = *filePtr

	// Load existing chat history if file exists.
	if _, err := os.Stat(markdownFilename); err == nil {
		content, err := ioutil.ReadFile(markdownFilename)
		if err != nil {
			log.Printf("failed to read markdown file: %v", err)
		} else {
			chatHistory = string(content)
		}
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		chatMutex.Lock()
		chatContent := chatHistory
		chatMutex.Unlock()
		data := struct {
			ChatHTML template.HTML
		}{
			ChatHTML: template.HTML(markdownToHTML(chatContent)),
		}
		if err := tmpl.Execute(w, data); err != nil {
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

// queryHandler processes each query, sends it to the Grokker API (simulated),
// updates the markdown file with the current chat state, and returns the LLM response as HTML.
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

	chatMutex.Lock()
	if req.Context != "" {
		chatHistory += fmt.Sprintf("**You:** %s:\n%s\n", req.Query, req.Context)
	} else {
		chatHistory += fmt.Sprintf("**You:** %s\n", req.Query)
	}
	updateMarkdown(markdownFilename, chatHistory)
	chatMutex.Unlock()

	// Call the LLM via grokker
	responseText := sendQueryToLLM(req.Query, req.LLM, req.Context)

	chatMutex.Lock()
	chatHistory += fmt.Sprintf("**Response:** %s\n", responseText)
	updateMarkdown(markdownFilename, chatHistory)
	chatMutex.Unlock()

	resp := QueryResponse{
		Response: markdownToHTML(responseText),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// sendQueryToLLM calls the Grokker API to obtain a markdown-formatted text.
func sendQueryToLLM(query string, llm string, context string) string {
	grok, _, _, _, lock, err := core.Load(llm, true)
	if err != nil {
		log.Printf("failed to load Grokker: %v", err)
		return fmt.Sprintf("failed to load Grokker: %v", err)
	}
	defer lock.Unlock()

	sysmsg := fmt.Sprintf("You are a helpful assistant. Respond to the query: %s", query)

	prompt := fmt.Sprintf("Query: %s\nContext: %s", query, context)

	msgs := []client.ChatMsg{
		{Role: "USER", Content: prompt},
	}

	var inputFiles []string
	var outFiles []core.FileLang

	fmt.Printf("Sending query to LLM '%s'\n", llm)
	response, _, err := grok.SendWithFiles(llm, sysmsg, msgs, inputFiles, outFiles)
	if err != nil {
		log.Printf("SendWithFiles error: %v", err)
		return fmt.Sprintf("Error sending query: %v", err)
	}
	fmt.Printf("Received response from LLM '%s'\n", llm)
	/*
		if context != "" {
			return fmt.Sprintf("Response for comment on '%s': %s [via %s]: %s", context, query, llm, response)
		}
		return fmt.Sprintf("Response from %s: %s: %s", llm, query, response)
	*/
	return response
}

// markdownToHTML converts markdown text to HTML using goldmark.
func markdownToHTML(markdown string) string {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(markdown), &buf); err != nil {
		log.Printf("Markdown conversion error: %v", err)
		return "<p>Error rendering markdown</p>"
	}
	return buf.String()
}

// updateMarkdown creates a backup of the existing markdown file and updates it with the new content.
func updateMarkdown(filename string, content string) (err error) {
	// write the old content to a backup file
	if oldContent, err := ioutil.ReadFile(filename); err == nil {
		backupName := filename + ".bak.md"
		if err := ioutil.WriteFile(backupName, oldContent, 0644); err != nil {
			log.Printf("failed to create backup: %v", err)
		}
	}

	// write the new content to a temporary file
	tempFile, err := ioutil.TempFile("", "storm-chat-*.md")
	if err != nil {
		log.Printf("failed to create temporary file: %v", err)
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	log.Printf("created temporary file %s", tempFile.Name())
	defer os.Remove(tempFile.Name()) // clean up temp file after writing

	if _, err := tempFile.WriteString(content); err != nil {
		log.Printf("failed to write to temporary file: %v", err)
		err = fmt.Errorf("failed to write to temporary file: %w", err)
		return err
	}
	tempFile.Close()

	// move the temporary file to the final destination
	if err := os.Rename(tempFile.Name(), filename); err != nil {
		log.Printf("failed to rename temporary file to %s: %v", filename, err)
		err = fmt.Errorf("failed to rename temporary file to %s: %w", filename, err)
		return err
	}
	log.Printf("updated markdown file %s", filename)

	return nil
}
