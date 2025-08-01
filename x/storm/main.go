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
	"strings"
	"sync"

	"github.com/gofrs/flock"
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
    #statusBox { display: inline-block; margin-left: 10px; vertical-align: middle; font-size: 9px; }
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
    <span id="statusBox">Token Count: 0</span>
  </div>
  <script>
    // Append a new message to the chat view without scrolling the page.
    function appendMessage(content) {
      var chat = document.getElementById("chat");
      var messageDiv = document.createElement("div");
      messageDiv.className = "message";
      messageDiv.innerHTML = content;
      // Instead of auto-scrolling or saving scroll position,
      // we simply append the content and let the browser handle it without scrolling.
      chat.appendChild(messageDiv);
    }

    // Send query to the /query endpoint.
    // Each query is immediately added to the chat with a 10px spinner.
    // When the LLM response is received the spinner is removed and replaced by the response.
    function sendQuery(query, llm, selection) {
      var chat = document.getElementById("chat");
      var messageDiv = document.createElement("div");
      messageDiv.className = "message";
	  if (selection === "") {
		  messageDiv.innerHTML = "<strong>" + query + "</strong>";
	  } else {
		  messageDiv.innerHTML = "<strong>" + query + " [" + selection + "]</strong>";
	  }
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
        body: JSON.stringify({ query: query, llm: llm, selection: selection })
      }).then(function(response) {
        return response.json();
      }).then(function(data) {
        // Remove the spinner once the response is received.
        spinner.remove();
        var responseDiv = document.createElement("div");
        responseDiv.innerHTML = data.response;
        messageDiv.appendChild(responseDiv);
		updateTokenCount();
      }).catch(function(err) {
        spinner.remove();
        var errorDiv = document.createElement("div");
        errorDiv.textContent = "Error: " + err;
        messageDiv.appendChild(errorDiv);
      });
    }

    // Poll the /tokencount endpoint to update the token count.
    function updateTokenCount() {
      fetch("/tokencount")
        .then(function(response) { return response.json(); })
        .then(function(data) {
          var statusBox = document.getElementById("statusBox");
          statusBox.textContent = "Token Count: " + data.tokens;
        })
        .catch(function(err) {
          console.error("Error fetching token count:", err);
        });
    }

	updateTokenCount(); // Initial token count fetch

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
			query = "Expand:";
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
	Query     string `json:"query"`
	LLM       string `json:"llm"`
	Selection string `json:"selection"`
}

// QueryResponse represents the LLM's response.
type QueryResponse struct {
	Response string `json:"response"`
}

// ChatRound contains a user query and its corresponding response.
type ChatRound struct {
	Query    string
	Response string
}

// Chat encapsulates chat history and synchronization.
type Chat struct {
	mutex    sync.Mutex
	history  []*ChatRound
	filename string
}

// NewChat creates a new Chat instance using the given markdown filename.
// If the file exists, its content is loaded as the initial chat history.
func NewChat(filename string) *Chat {
	var history []*ChatRound
	if _, err := os.Stat(filename); err == nil {
		content, err := ioutil.ReadFile(filename)
		if err != nil {
			log.Printf("failed to read markdown file: %v", err)
		} else {
			// Since the file stores the chat history as markdown,
			// we load it as a single ChatMessage with an empty query.
			history = append(history, &ChatRound{Response: string(content)})
		}
	}
	return &Chat{
		history:  history,
		filename: filename,
	}
}

// updateMarkdown creates a backup of the existing markdown file and updates it with the current chat history.
func (c *Chat) _updateMarkdown() error {

	// Convert the chat history slice into markdown content.
	// We don't need getHistory to lock, since we're already holding
	// the mutex, so 'false'.
	content := c.getHistory(false)

	// Write the old content to a backup file.
	if oldContent, err := ioutil.ReadFile(c.filename); err == nil {
		backupName := c.filename + ".bak.md"
		if err := ioutil.WriteFile(backupName, oldContent, 0644); err != nil {
			log.Printf("failed to create backup: %v", err)
		}
	}

	// Write the new content to a temporary file.
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

	// Move the temporary file to the final destination.
	if err := os.Rename(tempFile.Name(), c.filename); err != nil {
		log.Printf("failed to rename temporary file to %s: %v", c.filename, err)
		err = fmt.Errorf("failed to rename temporary file to %s: %w", c.filename, err)
		return err
	}
	log.Printf("updated markdown file %s", c.filename)

	return nil
}

// StartRound initializes a new chat round with a query and and empty response.
func (c *Chat) StartRound(query, selection string) (r *ChatRound) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	round := &ChatRound{}

	// trim the query to avoid leading/trailing whitespace
	q := strings.TrimSpace(query)

	// add selection if provided
	if selection != "" {
		q = fmt.Sprintf("%s:\n%s", q, selection)
	}

	round.Query = q
	c.history = append(c.history, round)

	log.Printf("started chat round: %s", query)
	return round
}

// FinishRound finalizes the current chat round with a response.
func (c *Chat) FinishRound(r *ChatRound, response string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if r == nil {
		return fmt.Errorf("cannot finish a nil chat round")
	}

	// update the response
	r.Response = response

	err := c._updateMarkdown()
	if err != nil {
		log.Printf("error updating markdown: %v", err)
		return fmt.Errorf("error updating markdown: %w", err)
	}

	log.Printf("finished chat round: %s", r.Query)
	return nil
}

// getHistory returns the current chat history as a markdown formatted string.
func (c *Chat) getHistory(lock bool) string {
	if lock {
		c.mutex.Lock()
		defer c.mutex.Unlock()
	}
	var result string
	for _, msg := range c.history {
		// skip rounds with empty responses -- they're still pending.
		if msg.Response == "" {
			continue
		}

		if msg.Query != "" {
			result += fmt.Sprintf("\n\n**%s**\n", msg.Query)
		}
		result += fmt.Sprintf("\n\n%s\n", msg.Response)
	}
	return result
}

var chat *Chat
var grok *core.Grokker

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	filePtr := flag.String("file", "", "markdown file to store chat history")
	flag.Parse()
	if *filePtr == "" {
		log.Fatal("must provide a markdown filename with -file")
	}

	var err error
	var lock *flock.Flock
	grok, _, _, _, lock, err = core.Load("", true)
	if err != nil {
		log.Fatalf("failed to load Grokker: %v", err)
	}
	defer lock.Unlock()

	chat = NewChat(*filePtr)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Received request for %s", r.URL.Path)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		chatContent := chat.getHistory(true)
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
	http.HandleFunc("/tokencount", tokenCountHandler)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Starting server on %s\n", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

// queryHandler processes each query, sends it to the Grokker API,
// updates the markdown file with the current chat state, and returns the LLM response as HTML.
func queryHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received query request: %s", r.URL.Path)
	if r.Method != "POST" {
		log.Printf("Method not allowed: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("error decoding request body: %v", err)
		http.Error(w, "Bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	round := chat.StartRound(req.Query, req.Selection)
	history := chat.getHistory(false)
	// add the last N characters of the chat history as context.
	// XXX should really use embeddings and a vector db to find relevant context.
	N := 150000
	startIndex := len(history) - N
	if startIndex < 0 {
		startIndex = 0
	}
	lastN := history[startIndex:]
	lastNTokenCount, err := grok.TokenCount(lastN)
	if err != nil {
		log.Printf("Token count error: %v", err)
		lastNTokenCount = 0
	}
	log.Printf("Added %d tokens of context to query: %s", lastNTokenCount, req.Query)

	responseText := sendQueryToLLM(req.Query, req.LLM, req.Selection, lastN)
	err = chat.FinishRound(round, responseText)
	if err != nil {
		http.Error(w, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := QueryResponse{
		Response: markdownToHTML(responseText),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// tokenCountHandler calculates the token count for the current conversation
// using the Grokker's TokenCount function and returns it as JSON.
func tokenCountHandler(w http.ResponseWriter, r *http.Request) {
	chatText := chat.getHistory(false)
	count, err := grok.TokenCount(chatText)
	if err != nil {
		log.Printf("Token count error: %v", err)
		count = 0
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"tokens": count})
}

// sendQueryToLLM calls the Grokker API to obtain a markdown-formatted text.
func sendQueryToLLM(query string, llm string, selection, backgroundContext string) string {
	sysmsg := fmt.Sprintf("You are a researcher.  I will start my prompt with some context, followed by a query.  Answer the query -- don't answer other questions you might see elsewhere in the context.")

	prompt := fmt.Sprintf("---CONTEXT START---\n%s\n---CONTEXT END---\n\nNew Query: %s [%s]", backgroundContext, query, selection)

	msgs := []client.ChatMsg{
		{Role: "USER", Content: prompt},
	}

	var inputFiles []string
	var outFiles []core.FileLang

	fmt.Printf("Sending query to LLM '%s'\n", llm)
	fmt.Printf("Query: %s\n", query)
	response, _, err := grok.SendWithFiles(llm, sysmsg, msgs, inputFiles, outFiles)
	if err != nil {
		log.Printf("SendWithFiles error: %v", err)
		return fmt.Sprintf("Error sending query: %v", err)
	}
	fmt.Printf("Received response from LLM '%s'\n", llm)
	fmt.Printf("Response: %s\n", response[:100]) // print first 100 characters of response
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
