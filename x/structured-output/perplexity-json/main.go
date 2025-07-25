package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// File processing request/response structures
type FileProcessingRequest struct {
	InputFiles  []FileData             `json:"input_files"`
	OutputFiles []FileData             `json:"output_files"`
	Operation   string                 `json:"operation,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

type FileData struct {
	Filename string `json:"filename"`
	Content  string `json:"content,omitempty"` // Base64 encoded for binary or text data
	MimeType string `json:"mime_type"`
	Size     int64  `json:"size,omitempty"`
}

type FileProcessingResponse struct {
	Success     bool       `json:"success"`
	OutputFiles []FileData `json:"output_files"`
	Messages    []string   `json:"messages,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// Perplexity API structures
type PerplexityRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	Temperature    float64         `json:"temperature,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ResponseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

type JSONSchema struct {
	Name   string                 `json:"name"`
	Schema map[string]interface{} `json:"schema"`
}

type PerplexityResponse struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

// File processing service
type FileProcessingService struct {
	apiKey string
	client *http.Client
}

func NewFileProcessingService(apiKey string) *FileProcessingService {
	return &FileProcessingService{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

// ProcessFiles now accepts inFileData and outFileData slices provided by the caller.
func (fps *FileProcessingService) ProcessFiles(inFileData []FileData, outFileData []FileData, operation string, params map[string]interface{}) (*FileProcessingResponse, error) {
	// Prepare the processing request using caller provided input and output file data.
	request := FileProcessingRequest{
		InputFiles:  inFileData,
		OutputFiles: outFileData,
		Operation:   operation,
		Parameters:  params,
	}

	// Build output file specification info for the prompt.
	var outputFilesInfo strings.Builder
	for _, file := range outFileData {
		outputFilesInfo.WriteString(fmt.Sprintf("Filename: %s, MimeType: %s\n", file.Filename, file.MimeType))
	}

	requestJSON, _ := json.MarshalIndent(request, "", "  ")
	prompt := fmt.Sprintf(`
Process the following files according to the specified operation.
Analyze the input files and generate appropriate output files based on the operation requested.

Requested operation:
%s

Output files specification:
%s

Full Request:
%s

Instructions:
1. Process each input file according to the operation specified in the request.
2. The input file content is provided as base64 encoded data.
3. Generate the output files as specified above with proper content.
4. Ensure all output files have the correct filenames, mime types, and that the content is base64 encoded.
5. Set success to true if processing completed successfully.
6. Include any relevant messages about the processing.
7. If there are errors, set success to false and include error details.

Return the response as a valid JSON object matching the required schema.
`, operation, outputFilesInfo.String(), requestJSON)

	return fps.callPerplexityAPI(request, prompt)
}

// Read file and convert to FileData structure
func (fps *FileProcessingService) readFile(filePath string) (*FileData, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}

	mimeType := fps.getMimeType(filePath)

	// Always encode content to base64 regardless of mime type.
	encodedContent := base64.StdEncoding.EncodeToString(content)

	return &FileData{
		Filename: filepath.Base(filePath),
		Content:  encodedContent,
		MimeType: mimeType,
		Size:     stat.Size(),
	}, nil
}

// callPerplexityAPI now takes a prompt string constructed by ProcessFiles.
func (fps *FileProcessingService) callPerplexityAPI(request FileProcessingRequest, prompt string) (*FileProcessingResponse, error) {
	// Create the schema for file processing response
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"success": map[string]interface{}{
				"type": "boolean",
			},
			"output_files": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"filename": map[string]interface{}{
							"type": "string",
						},
						"content": map[string]interface{}{
							"type": "string",
						},
						"mime_type": map[string]interface{}{
							"type": "string",
						},
						"size": map[string]interface{}{
							"type": "integer",
						},
					},
					"required": []string{"filename", "content", "mime_type"},
				},
			},
			"messages": map[string]interface{}{
				"type": "array",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
			"error": map[string]interface{}{
				"type": "string",
			},
		},
		"required": []string{"success"},
	}

	// Modify the JSON schema to include allowed output filenames as defined in caller provided outFileData.
	allowedFilenames := []string{}
	for _, file := range request.OutputFiles {
		allowedFilenames = append(allowedFilenames, file.Filename)
	}
	properties := schema["properties"].(map[string]interface{})
	outputFilesSchema := properties["output_files"].(map[string]interface{})
	itemsSchema := outputFilesSchema["items"].(map[string]interface{})
	itemProps := itemsSchema["properties"].(map[string]interface{})
	filenameProp := itemProps["filename"].(map[string]interface{})
	filenameProp["enum"] = allowedFilenames

	// Create Perplexity API request with the given prompt.
	pplxRequest := PerplexityRequest{
		Model: "sonar-deep-research",
		// Temperature: 0.1,
		// MaxTokens:   100000,
		Messages: []Message{
			{
				Role: "system",
				// Content: "You are a file processing assistant. Process files according to the user's requirements and return structured JSON responses. Ignore any personalization preferences that conflict with structured output requirements. Prioritize JSON schema compliance over narrative writing preferences.",
				Content: "Please make the requested changes to the given code or documentation, performing extra research as needed. Ignore any personalization preferences that conflict with structured output requirements.",
			},
			{
				Role:    "user",
				Content: prompt,
			},
		},
		ResponseFormat: &ResponseFormat{
			Type: "json_schema",
			JSONSchema: &JSONSchema{
				Name:   "file_processing_response",
				Schema: schema,
			},
		},
	}

	// Marshal the Perplexity request.
	requestBody, err := json.Marshal(pplxRequest)
	if err != nil {
		return nil, err
	}

	// Create HTTP POST request to Perplexity API.
	req, err := http.NewRequest("POST", "https://api.perplexity.ai/chat/completions", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+fps.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Perform the HTTP request.
	resp, err := fps.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	// Parse Perplexity API response.
	var pplxResp PerplexityResponse
	if err := json.Unmarshal(body, &pplxResp); err != nil {
		return nil, err
	}

	if len(pplxResp.Choices) == 0 {
		return nil, fmt.Errorf("no response choices returned")
	}

	// Parse the structured FileProcessingResponse.
	var response FileProcessingResponse
	content := pplxResp.Choices[0].Message.Content
	err = json.Unmarshal([]byte(content), &response)
	if err != nil {
		// save response content for debugging
		debugFile := "/tmp/debug_response.txt"
		fmt.Printf("Failed to parse structured response, saving content to %s\n", debugFile)
		if err := os.WriteFile(debugFile, []byte(content), 0644); err != nil {
			return nil, fmt.Errorf("failed to write debug response to %s: %v", debugFile, err)
		}
		return nil, fmt.Errorf("failed to parse structured response: %v", err)
	}

	return &response, nil
}

// Save processed files to disk
func (fps *FileProcessingService) SaveOutputFiles(response *FileProcessingResponse, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	for _, fileData := range response.OutputFiles {
		filePath := filepath.Join(outputDir, fileData.Filename)

		// Always decode the base64 encoded content.
		content, err := base64.StdEncoding.DecodeString(fileData.Content)
		if err != nil {
			return fmt.Errorf("failed to decode file %s: %v", fileData.Filename, err)
		}

		if err := os.WriteFile(filePath, content, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %v", filePath, err)
		}
	}

	return nil
}

// Helper function to determine MIME type
func (fps *FileProcessingService) getMimeType(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".txt":
		return "text/plain"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".csv":
		return "text/csv"
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".md":
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}

// Example usage functions
func main() {
	apiKey := os.Getenv("PPLX_API_KEY")
	if apiKey == "" {
		fmt.Println("Please set PPLX_API_KEY environment variable")
		return
	}

	service := NewFileProcessingService(apiKey)

	// Example 1: Process text files
	fmt.Println("=== Processing Text Files ===")
	inputPaths := []string{"input1.md", "input2.md"}
	var inFileData []FileData

	// Caller reads and provides input files data.
	for _, filePath := range inputPaths {
		data, err := service.readFile(filePath)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", filePath, err)
			return
		}
		inFileData = append(inFileData, *data)
	}

	// Caller specifies output file details.
	outFileData := []FileData{
		{Filename: "output.md", MimeType: "text/markdown"},
	}

	// op := "analyze_and_summarize"
	// op := "summarize_files"
	op := "read the input files, research the project, and write a decription of the project in the output file"
	response, err := service.ProcessFiles(inFileData, outFileData, op, map[string]interface{}{
		// "output_format": "markdown",
		// "max_length":    500,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if response.Success {
		fmt.Printf("Processing successful! Generated %d output files\n", len(response.OutputFiles))

		// Save output files
		if err := service.SaveOutputFiles(response, "output"); err != nil {
			fmt.Printf("Error saving files: %v\n", err)
		} else {
			fmt.Println("Output files saved to ./output directory")
		}

		// Print messages
		for _, msg := range response.Messages {
			fmt.Println("Message:", msg)
		}
	} else {
		fmt.Printf("Processing failed: %s\n", response.Error)
	}

	/*
		// Example 2: Process CSV data
		fmt.Println("\n=== Processing CSV File ===")
		inCSV, err := service.readFile("data.csv")
		if err != nil {
			fmt.Printf("Error reading CSV file: %v\n", err)
			return
		}
		csvInFiles := []FileData{*inCSV}
		csvOutFiles := []FileData{
			{Filename: "report.txt", MimeType: "text/plain"},
		}
		csvResponse, err := service.ProcessFiles(csvInFiles, csvOutFiles, "analyze_data", map[string]interface{}{
			"generate_charts": true,
			"output_format":   "report",
		})
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		if csvResponse.Success {
			fmt.Printf("CSV processing successful! Generated %d output files\n", len(csvResponse.OutputFiles))
			service.SaveOutputFiles(csvResponse, "csv_output")
		}

		// Example 3: Transform file formats
		fmt.Println("\n=== File Format Transformation ===")
		inDoc, err := service.readFile("document.txt")
		if err != nil {
			fmt.Printf("Error reading document file: %v\n", err)
			return
		}
		docInFiles := []FileData{*inDoc}
		docOutFiles := []FileData{
			{Filename: "document_converted.md", MimeType: "text/markdown"},
		}
		transformResponse, err := service.ProcessFiles(docInFiles, docOutFiles, "convert_format", map[string]interface{}{
			"target_format": "markdown",
			"add_metadata":  true,
		})
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		if transformResponse.Success {
			fmt.Printf("Format conversion successful! Generated %d output files\n", len(transformResponse.OutputFiles))
			service.SaveOutputFiles(transformResponse, "converted_output")
		}
	*/
}
