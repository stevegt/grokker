# Storm: Multi-Project LLM Chat Application

A single-daemon, single-port multi-project chat application for interacting with LLMs and local files. Storm provides both a command-line interface and web UI for managing projects, files, and conversations with language models.

## Features

- **Multi-Project Support**: Manage multiple independent projects with separate chat histories and file contexts
- **File Management**: Authorize and organize files for use as input context or output extraction
- **Unexpected Files Handling**: Automatic detection and user approval workflow for files returned by LLM but not explicitly requested
- **Web UI**: Real-time chat interface with file browser and progress tracking
- **CLI Interface**: Command-line tools for project and file management
- **WebSocket Communication**: Real-time bidirectional messaging between browser and server
- **Database Persistence**: SQLite-backed storage using BoltDB
- **Multi-Round Conversations**: Maintain chat history across multiple query rounds
- **Token Limit Management**: Configure token limits per query with preset shortcuts

## Quick Start

### Starting the Server

```bash
storm serve --port 8080
```

The server will start on http://localhost:8080

### Creating a Project

```bash
storm project add my-project /path/to/project /path/to/chat.md
```

### Managing Files

```bash
# Add files to a project
storm file add --project my-project /path/to/file1.go /path/to/file2.py

# List files in a project
storm file list --project my-project

# Remove files from a project
storm file forget --project my-project /path/to/file1.go
```

### Using the Web UI

1. Navigate to http://localhost:8080
2. Click on a project to open the chat interface
3. Click "Files" to manage input/output file selections
4. Type a query and click "Send" to interact with the LLM
5. Select an LLM provider from the dropdown (sonar-deep-research, sonar-reasoning, o3-mini)
6. Adjust token limits using presets or custom values

## Architecture

### Components

- **main.go**: Core server logic, HTTP routing, query processing
- **websocket.go**: Real-time WebSocket communication, client pool management
- **api.go**: RESTful API endpoints for project and file management
- **cli.go**: Command-line interface implementation
- **project.go**: Project registry and file management logic
- **db/**: Database abstraction layer with BoltDB implementation

### Data Flow

```
Browser UI
    ↓ (WebSocket query)
Server (processQuery)
    ↓ (HTTP to LLM)
LLM Service
    ↓ (response)
Server (ExtractFiles)
    ↓ (WebSocket response)
Browser UI (display results)
```

### File Selection Flow

1. User opens file modal in browser
2. Browser fetches authorized files from `/api/projects/{projectID}/files`
3. Browser displays file list with In/Out checkboxes
4. User selections stored in browser's IndexedDB
5. When sending query, selected files sent to server
6. Server resolves relative paths to absolute paths
7. Files passed to LLM as input/output context

### Unexpected Files Workflow

1. LLM returns code with files not in authorized list
2. Server detects unexpected files via `ExtractFiles()`
3. Server broadcasts `filesUpdated` message with unexpected files
4. Browser displays modal categorizing files (authorized vs new)
5. User approves files in modal
6. Browser sends `approveFiles` message with selected files
7. Server adds approved files to output extraction
8. Files are extracted and written to project directory

## Configuration

### Token Limits

Token limits apply to discussion response generation only.  There are
no limits on query or file size other than those imposed by the
selected LLM.

Default: 8192 tokens

Preset buttons available: 1K (1024), 2K (2048), 4K (4096), 8K (8192)

Custom values can be entered in the token limit input field.

### Database

Default location: `~/.storm/data.db`

Specify custom path with `--db-path` flag:
```bash
storm serve --port 8080 --db-path /custom/path/data.db
```

### LLM Providers

Configured via grokker library. Supported models include:
- sonar-deep-research
- sonar-reasoning
- o3-mini

## File Organization

Each project has:
- **Base Directory** (`baseDir`): Root directory containing project files
- **Markdown File** (`markdownFile`): Chat history stored in markdown format
- **Authorized Files**: List of files available as input/output context

Files can be:
- **Relative paths**: For files inside project `baseDir` 
- **Absolute paths**: Full filesystem paths for files outside `baseDir`

## API Endpoints

API docs are at `/docs` when server is running.

### Projects

- `POST /api/projects` - Create a project
- `GET /api/projects` - List all projects
- `DELETE /api/projects/{projectID}` - Delete a project

### Files

- `POST /api/projects/{projectID}/files/add` - Add files to project
- `GET /api/projects/{projectID}/files` - List authorized files
- `POST /api/projects/{projectID}/files/forget` - Remove files from project

### System

- `GET /api/version` - Get server version
- `POST /stop` - Gracefully shut down server

## WebSocket Messages

### Client → Server

**Query**:
```json
{
  "type": "query",
  "query": "user question",
  "llm": "sonar-deep-research",
  "selection": "optional context",
  "inputFiles": ["file1.go"],
  "outFiles": ["output.go"],
  "tokenLimit": 8192,
  "queryID": "uuid",
  "projectID": "project-id"
}
```

**Approve Files**:
```json
{
  "type": "approveFiles",
  "queryID": "uuid",
  "approvedFiles": ["new_file.go", "another_file.py"]
}
```

**Cancel Query**:
```json
{
  "type": "cancel",
  "queryID": "uuid"
}
```

### Server → Client

**Query Started**:
```json
{
  "type": "query",
  "query": "user question",
  "queryID": "uuid",
  "projectID": "project-id"
}
```

**Response**:
```json
{
  "type": "response",
  "queryID": "uuid",
  "response": "<html rendered markdown>",
  "projectID": "project-id"
}
```

**Files Updated**:
```json
{
  "type": "filesUpdated",
  "projectID": "project-id",
  "isUnexpectedFilesContext": true,
  "queryID": "uuid",
  "alreadyAuthorized": ["existing_file.go"],
  "needsAuthorization": ["new_file.go"],
  "files": ["file1.go", "file2.py"]
}
```

**Error**:
```json
{
  "type": "error",
  "queryID": "uuid",
  "message": "error description",
  "projectID": "project-id"
}
```

## Browser Storage

### IndexedDB

Storm uses IndexedDB to persist user selections:

- **Database**: `fileIODB`
- **Object Store**: `selections`
- **Records**: `{ filename, in: boolean, out: boolean }`

File selections are preserved across browser sessions within the same project.

## Performance Considerations

- **Token Counting**: Local operation using grokker library
- **File Extraction**: Dry-run first to check token limits, then real extraction on approval
- **WebSocket Messages**: Path normalization and sanitization at every broadcast
- **Database**: BoltDB provides ACID transactions for consistency

## Security

- **Path Validation**: All file paths converted to absolute and validated against project `baseDir`
- **Project Isolation**: Files and embeddings filtered by project permissions
- **WebSocket**: Path normalization enforced at message boundary ("relative on wire, absolute internally")
- **File Permissions**: BoltDB database stored with restrictive permissions (0600)

## Development

### Building

```bash
go build -o storm ./cmd/storm
```

### Testing

```bash
go test -v ./...
```

### Adding New LLM Providers

1. Ensure grokker library supports the provider
2. Add to `llmSelect` dropdown options in `project.html`
3. Select in UI and use via WebSocket

## Roadmap

- Vector database integration for semantic file search
- Multi-discussion file support per project
- Authentication and multi-user support
- OAuth integration for co-authored-by headers
- File change monitoring with inotify
- Queue feature for offline message batching

