package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"

	"storm/split"

	"github.com/gofrs/flock"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/client"
	"github.com/stevegt/grokker/v3/core"
	"github.com/stevegt/grokker/v3/util"
	"github.com/yuin/goldmark"
)

var tmpl = template.Must(template.New("index").Parse(`
<html>
<head>
  <meta charset="utf-8">
  <title>Grokker LLM Chat</title>
  <style>
    body { 
      font-family: Arial, sans-serif; 
      margin: 0; 
      padding: 0; 
      background-color: #121212; 
      color: #e0e0e0;
    }
    /* Container for sidebars and main content */
    #container { display: flex; height: 100vh; }
    /* Left sidebar for Table of Contents */
    #sidebar {
      width: 250px;
      background-color: #1e1e1e;
      border-right: 1px solid #333;
      overflow-y: auto;
      transition: width 0.3s;
      padding: 10px;
    }
    /* Collapsed sidebar style */
    #sidebar.collapsed {
      width: 10px;
      padding: 0;
      border: none;
      overflow: hidden;
    }
    /* Shrik the heading in the sidebar */
    #sidebar h3 { font-size: 0.9em; }
    /* Main content area */
    #main {
      flex: 1;
      display: flex;
      flex-direction: column;
      overflow: hidden;
    }
    /* Right sidebar for File I/O */
    #fileSidebar {
      width: 250px;
      background-color: #1e1e1e;
      border-left: 1px solid #333;
      padding: 10px;
      overflow-y: auto;
    }
    #fileSidebar h3 { margin-top: 0; }
    #fileSidebar table { width: 100%; border-collapse: collapse; }
    #fileSidebar th, #fileSidebar td { border: 1px solid #555; padding: 4px; text-align: center; }
    #fileSidebar input[type="text"] { width: 100%; margin-bottom: 5px; }
    /* Chat area styles */
    #chat { padding: 20px; flex: 1; overflow-y: auto; border-bottom: 1px solid #333; }
    .message { 
      margin-bottom: 10px; 
      padding: 5px; 
      border: 1px solid #444; 
      border-radius: 4px; 
      background-color: #252525; 
    }
    #spinner-area { padding: 10px; text-align: center; }
    .spinner {
      border: 4px solid #555;
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
    /* Updated input area using CSS Grid to span userInput and statusBox across two rows */
    #input-area { 
      background: #1e1e1e; 
      padding: 10px; 
      box-shadow: 0 -2px 5px rgba(0,0,0,0.1);
      display: grid;
      grid-template-areas: 
        "llmSelect userInput sendBtn statusBox stopBtn"
        "wordCount   userInput  .       statusBox .";
      grid-template-columns: auto 1fr auto auto auto;
      grid-template-rows: auto auto;
      gap: 5px;
    }
    textarea { 
      width: 100%; 
      height: 100%; 
      background-color: #333;
      color: #e0e0e0;
      border: 1px solid #555;
    }
    select { 
      background-color: #333;
      color: #e0e0e0;
      border: 1px solid #555;
    }
    input[type="number"] { 
      width: 80px; 
      height: 20px; 
      font-size: 12px; 
      padding: 5px; 
      background-color: #333;
      color: #e0e0e0;
      border: 1px solid #555;
    }
    button {
      background-color: #333;
      color: #e0e0e0;
      border: 1px solid #555;
      padding: 10px 15px;
      cursor: pointer;
    }
    button:hover {
      background-color: #444;
    }
    /* Custom style for the stop button to shrink its size and font */
    #stopBtn {
      font-size: 10px;
      padding: 5px 10px;
    }
    #statusBox { 
      display: inline-block; 
      font-size: 11px; 
    }
    /* Red stop sign for error indication in status box */
    #errorSign {
      display: none;
      color: red;
      font-size: 16px;
      margin-left: 5px;
    }
    /* Toggle button for sidebar */
    #toggle-sidebar {
      background-color: #3498db;
      color: #e0e0e0;
      border: 1px solid #555;
      padding: 5px 10px;
      cursor: pointer;
      margin-bottom: 10px;
      position: sticky;
      top: 0;
      z-index: 100;
    }
    /* Table of Contents links */
    #toc a {
      text-decoration: none;
      color: #ddd;
      padding: 4px;
      display: block;
    }
    #toc a:hover {
      background-color: #444;
    }
    /* Dark scrollbar styles */
    ::-webkit-scrollbar {
      width: 12px;
      height: 12px;
    }
    ::-webkit-scrollbar-track {
      background: #1e1e1e;
    }
    ::-webkit-scrollbar-thumb {
      background-color: #444;
      border: 2px solid #1e1e1e;
      border-radius: 6px;
    }
  </style>
</head>
<body>
  <div id="container">
    <div id="sidebar">
      <button id="toggle-sidebar">TOC</button>
      <h3>Table of Contents</h3>
      <div id="toc">
        <!-- TOC will be generated here -->
      </div>
    </div>
    <div id="main">
      <div id="chat">
        <!-- Chat messages will appear here -->
        {{.ChatHTML}}
      </div>
      <div id="spinner-area">
        <!-- Progress spinners will appear here -->
      </div>
      <div id="input-area">
        <select id="llmSelect" style="grid-area: llmSelect;">
          <option value="sonar-deep-research">sonar-deep-research</option>
          <option value="sonar-reasoning">sonar-reasoning</option>
          <option value="o3-mini">o3-mini</option>
        </select>
        <textarea id="userInput" placeholder="Enter query" style="grid-area: userInput;"></textarea>
        <button id="sendBtn" style="grid-area: sendBtn;">Send</button>
        <span id="statusBox" style="grid-area: statusBox;">
          <span id="tokenCountText">Token Count: 0</span>
          <br>
          <span id="roundsStats">Rounds:</span>
          <br>
          <span id="progressStats">Progress:</span>
          <br>
          <span id="statusSpinner" style="display:none;" class="spinner"></span>
          <span id="errorSign">⛔</span>
        </span>
        <button id="stopBtn" style="grid-area: stopBtn;">Stop<br>Server</button>
        <div id="wordCountContainer" style="grid-area: wordCount;">
          <label for="wordCount">Word Count</label>
          <input type="number" id="wordCount" min="1" placeholder="100">
          <div id="presetButtons">
            <button type="button" class="preset-wordcount" data-word="100" style="font-size:10px; padding:2px 5px; margin:2px;">100</button>
            <button type="button" class="preset-wordcount" data-word="300" style="font-size:10px; padding:2px 5px; margin:2px;">300</button>
            <button type="button" class="preset-wordcount" data-word="500" style="font-size:10px; padding:2px 5px; margin:2px;">500</button>
            <button type="button" class="preset-wordcount" data-word="700" style="font-size:10px; padding:2px 5px; margin:2px;">700</button>
          </div>
        </div>
      </div>
    </div>
    <div id="fileSidebar">
      <h3>Files</h3>
      <table>
        <thead>
          <tr>
            <th>In</th>
            <th>Out</th>
            <th>Filename</th>
            <th>Remove</th>
          </tr>
        </thead>
        <tbody id="fileList">
          <!-- File list will be rendered here -->
        </tbody>
      </table>
      <div id="newFileEntry">
        <label><input type="checkbox" id="newFileIn"> In</label>
        <label><input type="checkbox" id="newFileOut"> Out</label>
        <input type="text" id="newFilename" placeholder="New filename">
        <button id="addFileBtn">Add</button>
      </div>
    </div>
  </div>
  <script>
    // Helper functions for managing cookies.
    function setCookie(name, value, days) {
      var expires = "";
      if (days) {
        var date = new Date();
        date.setTime(date.getTime() + (days*24*60*60*1000));
        expires = "; expires=" + date.toUTCString();
      }
      document.cookie = name + "=" + (value || "")  + expires + "; path=/";
    }
    function getCookie(name) {
      var nameEQ = name + "=";
      var ca = document.cookie.split(';');
      for(var i=0; i < ca.length; i++) {
        var c = ca[i].trim();
        if (c.indexOf(nameEQ) === 0) return c.substring(nameEQ.length, c.length);
      }
      return null;
    }

    // Global counter for outstanding queries.
    var outstandingQueries = 0;
    // Updates the spinner in the status box based on the current outstanding query count.
    function updateStatusSpinner() {
      var spinner = document.getElementById("statusSpinner");
      if (outstandingQueries > 0) {
        spinner.style.display = "inline-block";
      } else {
        spinner.style.display = "none";
      }
    }

    // Show the error stop sign. Once shown, it remains visible until the page is reloaded.
    function showErrorSign() {
      var errorSign = document.getElementById("errorSign");
      if (errorSign) {
        errorSign.style.display = "inline-block";
      }
    }

    // Generate a Table of Contents from headings in the chat
    function generateTOC() {
      var chat = document.getElementById("chat");
      var headings = chat.querySelectorAll("h1, h2, h3, h4, h5, h6");
      var toc = document.getElementById("toc");
      toc.innerHTML = "";
      headings.forEach(function(heading, index) {
        if (!heading.id) {
          heading.id = "heading-" + index;
        }
        // Determine heading level and create link with indentation and font size
        var level = parseInt(heading.tagName.substring(1));
        var link = document.createElement("a");
        link.href = "#" + heading.id;
        link.textContent = heading.textContent;
        // Bold top-level links (h1)
        if(level === 1) {
          link.style.fontWeight = "bold";
        }
        // Indent based on level, e.g. 20px per sub-level
        link.style.marginLeft = ((level - 1) * 20) + "px";
        // Adjust font size based on heading level (shrunk from original values)
        var fontSize = Math.max(1.0 - 0.1 * (level - 1), 0.7);
        link.style.fontSize = fontSize + "em";
        toc.appendChild(link);
      });
    }
    // Call generateTOC and other initializations when the DOM content is loaded.
    document.addEventListener("DOMContentLoaded", function() {
      generateTOC();
      // Toggle sidebar visibility
      var sidebar = document.getElementById("sidebar");
      document.getElementById("toggle-sidebar").addEventListener("click", function() {
        if (sidebar.classList.contains("collapsed")) {
          sidebar.classList.remove("collapsed");
        } else {
          sidebar.classList.add("collapsed");
        }
      });
      // Add preset word count buttons functionality.
      document.querySelectorAll('.preset-wordcount').forEach(function(btn) {
        btn.addEventListener('click', function() {
          document.getElementById('wordCount').value = this.getAttribute('data-word');
        });
      });
      // scroll to the bookmarked round 
      var bookmark = getCookie("bookmark_round");
      if (bookmark) {
        var round = parseInt(bookmark);
        var chat = document.getElementById("chat");
        var hrTags = chat.getElementsByTagName("hr");
        if (round > 0 && round <= hrTags.length) {
          console.log("Scrolling to round:", round);
          chat.scrollTop = hrTags[round - 1].offsetTop;
        }
      }
      updateProgressStats();
      initFileIO();
    });

    // Append a new message to the chat view without scrolling the page.
    function appendMessage(content) {
      var chat = document.getElementById("chat");
      var messageDiv = document.createElement("div");
      messageDiv.className = "message";
      messageDiv.innerHTML = content;
      // Instead of auto-scrolling or saving scroll position,
      // we simply append the content and let the browser handle it without scrolling.
      chat.appendChild(messageDiv);
      generateTOC();
    }

    // Send query to the /query endpoint.
    // Each query is immediately added to the chat with a 10px spinner and a Cancel button.
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
      // create a <br> before the spinner and cancel button
      messageDiv.innerHTML += "<br>";
      // Create a spinner element next to the query.
      var spinner = document.createElement("span");
      spinner.className = "spinner";
      spinner.style.marginLeft = "10px";
      messageDiv.appendChild(spinner);
      // Create a Cancel button next to the spinner.
      var cancelBtn = document.createElement("button");
      cancelBtn.textContent = "Cancel";
      cancelBtn.style.marginLeft = "5px";
      messageDiv.appendChild(cancelBtn);
      chat.appendChild(messageDiv);
      generateTOC();

      // Increment global outstanding query count and update status spinner.
      outstandingQueries++;
      updateStatusSpinner();

      // Create an abort controller to cancel the fetch request.
      var abortController = new AbortController();

      // When the user clicks the Cancel button, abort the request and remove the message.
      cancelBtn.addEventListener("click", function() {
        abortController.abort();
        messageDiv.remove();
        // Decrement outstanding queries and update status spinner when cancelled.
        outstandingQueries--;
        updateStatusSpinner();
        generateTOC();
      });

      // Gather file I/O selections from the file sidebar.
      var fileSelection = getSelectedFiles();

      fetch("/query", {
        method: "POST",
        headers: {
          "Content-Type": "application/json"
        },
        signal: abortController.signal,
        body: JSON.stringify({ 
          query: query, 
          llm: llm, 
          selection: selection,
          inputFiles: fileSelection.inputFiles,
          outFiles: fileSelection.outFiles
        })
      }).then(function(response) {
        return response.json();
      }).then(function(data) {
        // Remove the spinner and cancel button once the response is received.
        spinner.remove();
        cancelBtn.remove();
        var responseDiv = document.createElement("div");
        responseDiv.innerHTML = data.response;
        // If the messageDiv is still in the document (i.e. not cancelled), append the response.
        if (document.body.contains(messageDiv)) {
          messageDiv.appendChild(responseDiv);
          updateTokenCount();
          generateTOC();
          updateProgressStats();
        }
        // Decrement outstanding queries and update status spinner.
        outstandingQueries--;
        updateStatusSpinner();
      }).catch(function(err) {
        if (err.name === "AbortError") {
          return;
        }
        spinner.remove();
        cancelBtn.remove();
        var errorDiv = document.createElement("div");
        errorDiv.textContent = "Error: " + err;
        messageDiv.appendChild(errorDiv);
        // Show red stop sign in the status box in case of error.
        showErrorSign();
        // Decrement outstanding queries and update status spinner.
        outstandingQueries--;
        updateStatusSpinner();
      });
    }

    // Poll the /tokencount endpoint to update the token count.
    function updateTokenCount() {
      fetch("/tokencount")
        .then(function(response) { return response.json(); })
        .then(function(data) {
          var tokenCountText = document.getElementById("tokenCountText");
          tokenCountText.textContent = "Token Count: " + data.tokens;
        })
        .catch(function(err) {
          console.error("Error fetching token count:", err);
        });
    }

    // Updates progress stats by counting the number of <hr> tags above the current scroll position
    // and fetching the total round count from the server.
    function updateProgressStats() {
      var chatElem = document.getElementById("chat");
      var hrTags = chatElem.getElementsByTagName("hr");
      var currentRound = 0;
      // Count the number of <hr> tags that are above the current scroll top
      for (var i = 0; i < hrTags.length; i++) {
        var hrPos = hrTags[i].offsetTop;
        if (hrPos < chatElem.scrollTop) {
          currentRound++;
        }
      }
      // Bookmark the current round in a cookie (for one year)
      setCookie("bookmark_round", currentRound, 365);
      fetch("/rounds")
        .then(function(response) { return response.json(); })
        .then(function(data) {
          var total = data.rounds;
          var remaining = total - currentRound;
          var percentage = total > 0 ? Math.round((currentRound / total) * 100) : 0;
          var roundsElem = document.getElementById("roundsStats");
          var progressElem = document.getElementById("progressStats");
          if(roundsElem) {
            // Rounds: total - current = remaining 
            roundsElem.textContent = "Rounds: " + total + " - " + currentRound + " = " + remaining;
          }
          if(progressElem) {
            // Progress: N%
            progressElem.textContent = "Progress: " + percentage + "%";
          }
        })
        .catch(function(err) {
          console.error("Error fetching rounds count:", err);
        });
    }

    // Add scroll event listener on the chat element to update progress stats and update bookmark.
    document.getElementById("chat").addEventListener("scroll", updateProgressStats);
    updateTokenCount(); // Initial token count fetch

    // Handle click on the Send button.
    document.getElementById("sendBtn").addEventListener("click", function() {
      var input = document.getElementById("userInput");
      var query = input.value;
      if(query.trim() === "") return;
      // Check for word count input
      var wordCountElem = document.getElementById("wordCount");
      if(wordCountElem) {
        if(wordCountElem.value.trim() == "") {
          // default to 100 words if no input is given
          wordCountElem.value = "100";
        }
        query = query + "\n\nPlease limit your response to " + wordCountElem.value + " words.";
      }
      sendQuery(query, document.getElementById("llmSelect").value, "");
      input.value = "";
      // Do not clear the word count input so the value persists.
    });

    // Handle click on the Stop Server button.
    document.getElementById("stopBtn").addEventListener("click", function() {
      if(confirm("Are you sure you want to stop the server?")) {
        fetch("/stop", { method: "POST" })
          .then(function(response) {
            if(response.ok) {
              console.log("Server is stopping...");
            }
          })
          .catch(function(err) {
            console.error("Error stopping server:", err);
          });
      }
    });

    // --- File I/O using IndexedDB ---
    var db;
    function initFileIO() {
      var request = indexedDB.open("fileIODB", 1);
      request.onerror = function(event) {
        console.error("IndexedDB error:", event.target.error);
      };
      request.onupgradeneeded = function(event) {
        db = event.target.result;
        if (!db.objectStoreNames.contains("files")) {
          var store = db.createObjectStore("files", { keyPath: "filename" });
          store.createIndex("by_filename", "filename", { unique: true });
        }
      };
      request.onsuccess = function(event) {
        db = event.target.result;
        loadFileList();
      };
    }
    function loadFileList() {
      var transaction = db.transaction(["files"], "readonly");
      var store = transaction.objectStore("files");
      var request = store.getAll();
      request.onsuccess = function(event) {
        var files = event.target.result;
        renderFileList(files);
      };
    }
    function saveFileEntry(fileEntry) {
      var transaction = db.transaction(["files"], "readwrite");
      var store = transaction.objectStore("files");
      store.put(fileEntry);
    }
    function removeFileEntry(filename) {
      var transaction = db.transaction(["files"], "readwrite");
      var store = transaction.objectStore("files");
      var request = store.delete(filename);
      request.onsuccess = function(event) {
        loadFileList();
      };
      request.onerror = function(event) {
        console.error("Failed to delete file:", filename);
      };
    }
    function renderFileList(files) {
      var fileListElem = document.getElementById("fileList");
      fileListElem.innerHTML = "";
      files.forEach(function(file) {
        var tr = document.createElement("tr");
        var tdIn = document.createElement("td");
        var inCheckbox = document.createElement("input");
        inCheckbox.type = "checkbox";
        inCheckbox.checked = file.in || false;
        inCheckbox.className = "fileIn";
        inCheckbox.addEventListener("change", function() {
          file.in = inCheckbox.checked;
          saveFileEntry(file);
        });
        tdIn.appendChild(inCheckbox);
        var tdOut = document.createElement("td");
        var outCheckbox = document.createElement("input");
        outCheckbox.type = "checkbox";
        outCheckbox.checked = file.out || false;
        outCheckbox.className = "fileOut";
        outCheckbox.addEventListener("change", function() {
          file.out = outCheckbox.checked;
          saveFileEntry(file);
        });
        tdOut.appendChild(outCheckbox);
        var tdName = document.createElement("td");
        var link = document.createElement("a");
        link.href = "/open?filename=" + encodeURIComponent(file.filename);
        link.target = "_blank";
        link.textContent = file.filename;
        tdName.appendChild(link);
        tr.appendChild(tdIn);
        tr.appendChild(tdOut);
        tr.appendChild(tdName);
        var tdRemove = document.createElement("td");
        var removeBtn = document.createElement("button");
        removeBtn.textContent = "x";
        removeBtn.title = "Remove this file from the list";
        removeBtn.style.fontSize = "10px";
        removeBtn.style.padding = "2px 5px";
        removeBtn.addEventListener("click", function() {
          removeFileEntry(file.filename);
        });
        tdRemove.appendChild(removeBtn);
        tr.appendChild(tdRemove);
        fileListElem.appendChild(tr);
      });
    }
    document.getElementById("addFileBtn").addEventListener("click", function() {
      var newFilename = document.getElementById("newFilename").value.trim();
      if(newFilename === "") return;
      var newFileEntry = {
        filename: newFilename,
        in: document.getElementById("newFileIn").checked,
        out: document.getElementById("newFileOut").checked
      };
      saveFileEntry(newFileEntry);
      loadFileList();
      document.getElementById("newFilename").value = "";
      document.getElementById("newFileIn").checked = false;
      document.getElementById("newFileOut").checked = false;
    });
    function getSelectedFiles() {
      var inputFiles = [];
      var outFiles = [];
      var rows = document.getElementById("fileList").getElementsByTagName("tr");
      for (var i = 0; i < rows.length; i++) {
        var cells = rows[i].getElementsByTagName("td");
        if(cells.length < 3) continue;
        var inChecked = cells[0].querySelector("input").checked;
        var outChecked = cells[1].querySelector("input").checked;
        var filename = cells[2].textContent;
        if(inChecked) inputFiles.push(filename);
        if(outChecked) outFiles.push(filename);
      }
      return { inputFiles: inputFiles, outFiles: outFiles };
    }
    // --- End File I/O code ---
  </script>
</body>
</html>
`))

// QueryRequest represents a user's query input.
type QueryRequest struct {
	Query      string   `json:"query"`
	LLM        string   `json:"llm"`
	Selection  string   `json:"selection"`
	InputFiles []string `json:"inputFiles"`
	OutFiles   []string `json:"outFiles"`
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

			// load the markdown file and parse it into chat rounds.
			roundTrips, err := split.Parse(bytes.NewReader(content))
			Ck(err)
			for _, rt := range roundTrips {
				response := Spf("%s\n\n## References\n\n%s\n\n## Reasoning\n\n%s\n\n", rt.Response, rt.References, rt.Reasoning)
				chatRound := &ChatRound{
					Query:    rt.Query,
					Response: response,
				}
				history = append(history, chatRound)
			}
		}
	}
	return &Chat{
		history:  history,
		filename: filename,
	}
}

// TotalRounds returns the total number of chat rounds.
func (c *Chat) TotalRounds() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return len(c.history)
}

// _updateMarkdown writes the current chat history to the markdown file.
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
	defer os.Remove(tempFile.Name())
	if _, err := tempFile.WriteString(content); err != nil {
		log.Printf("failed to write to temporary file: %v", err)
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}
	tempFile.Close()
	if err := os.Rename(tempFile.Name(), c.filename); err != nil {
		log.Printf("failed to rename temporary file to %s: %v", c.filename, err)
		return fmt.Errorf("failed to rename temporary file to %s: %w", c.filename, err)
	}
	log.Printf("updated markdown file %s", c.filename)
	return nil
}

// StartRound initializes a chat round.
func (c *Chat) StartRound(query, selection string) (r *ChatRound) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	round := &ChatRound{}
	q := strings.TrimSpace(query)
	if selection != "" {
		q = fmt.Sprintf("%s: [%s]", q, selection)
	}
	round.Query = q
	c.history = append(c.history, round)
	log.Printf("started chat round: %s", query)
	return round
}

// FinishRound finalizes a chat round.
func (c *Chat) FinishRound(r *ChatRound, response string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if r == nil {
		return fmt.Errorf("cannot finish a nil chat round")
	}
	r.Response = response
	err := c._updateMarkdown()
	if err != nil {
		log.Printf("error updating markdown: %v", err)
		return fmt.Errorf("error updating markdown: %w", err)
	}
	log.Printf("finished chat round: %s", r.Query)
	return nil
}

// getHistory returns the chat history as markdown.
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
		result += fmt.Sprintf("\n\n%s\n\n---\n\n", msg.Response)
	}
	return result
}

var chat *Chat
var grok *core.Grokker
var srv *http.Server

func main() {
	fmt.Println("storm v0.0.75")
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
	http.HandleFunc("/rounds", roundsHandler)
	http.HandleFunc("/stop", stopHandler)
	http.HandleFunc("/open", openHandler)

	addr := fmt.Sprintf(":%d", *port)
	srv = &http.Server{Addr: addr}
	log.Printf("Starting server on %s\n", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// openHandler serves a file based on the filename query parameter.
func openHandler(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		http.Error(w, "Missing filename parameter", http.StatusBadRequest)
		return
	}
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}
	http.ServeFile(w, r, filename)
}

// stopHandler gracefully shuts down the server.
func stopHandler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Received stop server request: %s", r.URL.Path)
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Server stopping"))
	// Shutdown the server gracefully in a separate goroutine.
	go func() {
		if err := srv.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down server: %v", err)
		}
	}()
}

// roundsHandler returns the total number of chat rounds as JSON.
func roundsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	rounds := chat.TotalRounds()
	json.NewEncoder(w).Encode(map[string]int{"rounds": rounds})
}

var TailLength = 300000

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
	// add the last TailLength characters of the chat history as context.
	// XXX should really use embeddings and a vector db to find relevant context.
	startIndex := len(history) - TailLength
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

	responseText := sendQueryToLLM(req.Query, req.LLM, req.Selection, lastN, req.InputFiles, req.OutFiles)

	// convert references to a bulleted list
	refIndex := strings.Index(responseText, "<references>")
	if refIndex != -1 {
		refEndIndex := strings.Index(responseText, "</references>") + len("</references>")
		// every non-blank line after <references> is a reference --
		// insert a '- ' before each line until we hit the closing tag.
		firstRefIndex := refIndex + len("<references>")
		references := strings.Split(responseText[firstRefIndex:], "\n")
		var refLines []string
		for _, line := range references {
			line = strings.TrimSpace(line)
			if line == "</references>" {
				break // stop at the closing tag
			}
			if line == "" {
				continue // skip empty lines
			}

			// if the line looks like [N] followed by a URL, convert
			// the URL to a markdown link.
			regex := `^\s*\[(\d+)\]\s*(http[s]?://\S+)\s*$`
			re := regexp.MustCompile(regex)
			m := re.FindStringSubmatch(line)
			if len(m) > 0 {
				// m[1] is the reference number, m[2] is the URL
				line = fmt.Sprintf("- [%s] [%s](%s)", m[1], m[2], m[2])
			}

			refLines = append(refLines, line)
		}
		// replace the original <references> section with the new ## References section.
		beforeRefs := responseText[:refIndex]
		refHead := "\n\n## References\n\n"
		afterRefs := responseText[refEndIndex:]
		responseText = beforeRefs + refHead + strings.Join(refLines, "\n") + "\n" + afterRefs
	}

	// move the <think> section to the end of the response
	thinkIndex := strings.Index(responseText, "<think>")
	if thinkIndex != -1 {
		thinkEndIndex := strings.Index(responseText, "</think>") + len("</think>")
		if thinkEndIndex > thinkIndex {
			thinkSection := responseText[thinkIndex:thinkEndIndex]
			// remove the think section from the response
			responseText = responseText[:thinkIndex] + responseText[thinkEndIndex:]
			// append the think section to the end of the response
			responseText += "\n\n" + thinkSection
		} else {
			log.Printf("Malformed <think> section in response: %s", responseText)
		}
	}
	// convert <think> tags to a markdown heading
	replacer := strings.NewReplacer("<think>", "## Reasoning\n", "</think>", "")
	responseText = replacer.Replace(responseText)

	err = chat.FinishRound(round, responseText)
	if err != nil {
		http.Error(w, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp := QueryResponse{
		Response: markdownToHTML(responseText) + "\n\n<hr>\n\n",
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// tokenCountHandler calculates the token count for the current conversation
// using Grokker's TokenCount function and returns it as JSON.
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
func sendQueryToLLM(query string, llm string, selection, backgroundContext string, inputFiles []string, outFiles []string) string {
	sysmsg := "You are a researcher.  I will start my prompt with some context, followed by a query.  Answer the query -- don't answer other questions you might see elsewhere in the context.  Always enclose reference numbers in square brackets; ignore empty brackets in the prompt or context, and DO NOT INCLUDE EMPTY SQUARE BRACKETS in your response, regardless of what you see in the context.  Always start your response with a markdown heading."

	prompt := fmt.Sprintf("---CONTEXT START---\n%s\n---CONTEXT END---\n\nNew Query: %s", backgroundContext, query)
	if selection != "" {
		prompt += fmt.Sprintf(" {%s}", selection)
	}

	msgs := []client.ChatMsg{
		{Role: "USER", Content: prompt},
	}
	var outFilesConverted []core.FileLang
	for _, f := range outFiles {
		lang, known, err := util.Ext2Lang(f)
		Ck(err)
		if !known {
			log.Printf("Unknown file extension for output file %s; assuming language is %s", f, lang)
		}
		outFilesConverted = append(outFilesConverted, core.FileLang{File: f, Language: lang})
	}
	fmt.Printf("Sending query to LLM '%s'\n", llm)
	fmt.Printf("Query: %s\n", query)
	response, _, err := grok.SendWithFiles(llm, sysmsg, msgs, inputFiles, outFilesConverted)
	if err != nil {
		log.Printf("SendWithFiles error: %v", err)
		return fmt.Sprintf("Error sending query: %v", err)
	}
	fmt.Printf("Received response from LLM '%s'\n", llm)
	fmt.Printf("Response: %s\n", response)

	cookedResponse, err := core.ExtractFiles(outFilesConverted, response, core.ExtractOptions{
		DryRun:             false,
		ExtractToStdout:    false,
		RemoveFromResponse: true,
	})

	return cookedResponse
}

// splitMarkdown splits the markdown input into sections separated by a horizontal rule (^---$).
func splitMarkdown(input string) []string {
	re := regexp.MustCompile("(?m)^---$")
	sections := re.Split(input, -1)
	return sections
}

// collectReferences scans the markdown input for reference lines of the form "- [N] URL"
// and returns a map of URLs keyed by the reference number.
func collectReferences(input string) map[string]string {
	re := regexp.MustCompile(`(?m)^-\s+\[(\d+)\]\s+\[(http[s]?://\S+)\]`)
	matches := re.FindAllStringSubmatch(input, -1)
	refs := make(map[string]string)
	for _, m := range matches {
		if len(m) == 3 {
			refs[m[1]] = m[2]
		}
	}
	return refs
}

// linkifyReferences replaces occurrences of references like "[N]" with markdown links to the corresponding URL.
func linkifyReferences(input string, refs map[string]string) string {
	re := regexp.MustCompile(`\[(\d+)\]`)
	result := re.ReplaceAllStringFunc(input, func(match string) string {
		m := re.FindStringSubmatch(match)
		if len(m) == 2 {
			url, ok := refs[m[1]]
			if ok && m[1] != "" && url != "" {
				return fmt.Sprintf("[[%s](%s)]", m[1], url)
			}
		}
		return match
	})
	return result
}

// markdownToHTML converts markdown text to HTML using goldmark.
// It first splits the markdown into sections, collects any reference URLs, and replaces each "[N]"
// with a markdown link to the corresponding URL before rendering.
func markdownToHTML(markdown string) string {

	// linkify references in the markdown
	sections := splitMarkdown(markdown)
	for i, sec := range sections {
		refs := collectReferences(sec)
		// log.Printf("Found %d references in section %d", len(refs), i)
		sections[i] = linkifyReferences(sec, refs)
	}
	processed := strings.Join(sections, "\n\n---\n\n")

	/*
		// replace '^---$' with an HTML horizontal rule
		pattern := regexp.MustCompile("(?m)^---$")
		processed = pattern.ReplaceAllString(processed, "<hr>")
	*/

	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(processed), &buf); err != nil {
		log.Printf("Markdown conversion error: %v", err)
		return "<p>Error rendering markdown</p>"
	}

	return buf.String()
}

