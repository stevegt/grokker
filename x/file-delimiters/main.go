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
	"regexp"
	"strings"
)

// File processing request/response structures (simplified for delimiter-based approach)
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

// Perplexity API structures (simplified - no ResponseFormat needed)
type PerplexityRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type PerplexityResponse struct {
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

// Enhanced file processing service with delimiter-based parsing
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

// ProcessFiles now uses delimiter-based output instead of JSON structured output
func (fps *FileProcessingService) ProcessFiles(inFileData []FileData, outFileData []FileData, operation string, params map[string]interface{}) (*FileProcessingResponse, error) {
	// Prepare the processing request
	request := FileProcessingRequest{
		InputFiles:  inFileData,
		OutputFiles: outFileData,
		Operation:   operation,
		Parameters:  params,
	}

	// Build output file specification for the prompt
	var outputFilesInfo strings.Builder
	for _, file := range outFileData {
		outputFilesInfo.WriteString(fmt.Sprintf("Filename: %s, MimeType: %s\n", file.Filename, file.MimeType))
	}

	// Create delimiter-based prompt (NO JSON Schema)
	prompt := fps.buildDelimiterBasedPrompt(request, outputFilesInfo.String())

	// Process via Perplexity API using standard text output
	response, err := fps.callPerplexityAPIWithDelimiters(prompt)
	if err != nil {
		return &FileProcessingResponse{
			Success: false,
			Error:   fmt.Sprintf("API call failed: %v", err),
		}, err
	}

	return response, nil
}

// Build delimiter-based prompt that instructs the AI to use your single-line delimiter format
func (fps *FileProcessingService) buildDelimiterBasedPrompt(request FileProcessingRequest, outputFilesInfo string) string {
	// Build detailed prompt with your single-line delimiter specification
	prompt := fmt.Sprintf(`
Process the following files according to the specified operation and generate output files using EXACTLY the delimiter format specified below.

Operation: %s
Output files specification:
%s

CRITICAL OUTPUT FORMAT REQUIREMENTS:
You MUST use this EXACT delimiter format for each output file:

---FILE-START filename="exact_filename.ext"---
[base64 encoded file content here]
---FILE-END filename="exact_filename.ext"---

IMPORTANT RULES:
1. Each delimiter must be on its own line
2. Use the EXACT filename specified in the output files specification
3. Content must be base64 encoded
4. Include success/error messages after all files using:
---PROCESSING-RESULT---
Success: true/false
Messages: Any processing messages
Error: Any error details (if applicable)
---END-RESULT---

Input files provided:
`, request.Operation, outputFilesInfo)

	// Add input file information
	for i, file := range request.InputFiles {
		prompt += fmt.Sprintf(`
Input File %d:
Filename: %s
Content: %s
MimeType: %s

`, i+1, file.Filename, file.Content, file.MimeType)
	}

	prompt += `
Now process these input files according to the operation and generate the output files using the delimiter format specified above.
`

	return prompt
}

// Call Perplexity API without JSON structured output - use regular text generation
func (fps *FileProcessingService) callPerplexityAPIWithDelimiters(prompt string) (*FileProcessingResponse, error) {
	// Create standard Perplexity API request (NO response_format)
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
		// NO ResponseFormat field - this allows free-form text output
	}

	// Execute the API call
	rawResponse, err := fps.executeAPICall(pplxRequest)
	if err != nil {
		return nil, err
	}

	// write raw response to a file for debugging
	debugFile := "/tmp/raw_response.txt"
	if err := os.WriteFile(debugFile, []byte(rawResponse), 0644); err != nil {
		return nil, fmt.Errorf("failed to write raw response to debug file: %v", err)
	}

	// Parse the delimiter-based response
	return fps.parseDelimiterResponse(rawResponse)
}

// Execute the API call and return raw response content
func (fps *FileProcessingService) executeAPICall(pplxRequest PerplexityRequest) (string, error) {
	// Marshal request
	requestBody, err := json.Marshal(pplxRequest)
	if err != nil {
		return "", err
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", "https://api.perplexity.ai/chat/completions", bytes.NewBuffer(requestBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+fps.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Make request
	resp, err := fps.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	// Parse Perplexity response to get content
	var pplxResp PerplexityResponse
	if err := json.Unmarshal(body, &pplxResp); err != nil {
		return "", err
	}

	if len(pplxResp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}

	return pplxResp.Choices[0].Message.Content, nil
}

// Parse delimiter-based response using robust regex patterns
func (fps *FileProcessingService) parseDelimiterResponse(response string) (*FileProcessingResponse, error) {
	// Create response object
	result := &FileProcessingResponse{
		Success:     false,
		OutputFiles: []FileData{},
		Messages:    []string{},
	}

	// Primary parsing: Use your single-line delimiter format
	files, err := fps.parseWithSingleLineDelimiters(response)
	if err == nil && len(files) > 0 {
		result.OutputFiles = files
		result.Success = true
		return result, nil
	}

	// Fallback parsing: Try to parse any delimiter-like patterns
	files, err = fps.parseWithFallbackDelimiters(response)
	if err == nil && len(files) > 0 {
		result.OutputFiles = files
		result.Success = true
		result.Messages = append(result.Messages, "Used fallback delimiter parsing")
		return result, nil
	}

	// Last resort: Try to extract any base64-like content
	files, err = fps.parseWithContentExtraction(response)
	if err == nil && len(files) > 0 {
		result.OutputFiles = files
		result.Success = true
		result.Messages = append(result.Messages, "Used content extraction parsing")
		return result, nil
	}

	// If all parsing methods fail
	result.Error = "Failed to parse any files from response"
	// Save raw response for debugging
	debugFile := "/tmp/unparseable_response.txt"
	os.WriteFile(debugFile, []byte(response), 0644)
	result.Messages = append(result.Messages, fmt.Sprintf("Raw response saved to %s for debugging", debugFile))

	return result, fmt.Errorf("could not parse response using any method")
}

// Parse using your specified single-line delimiter format
func (fps *FileProcessingService) parseWithSingleLineDelimiters(response string) ([]FileData, error) {
	var files []FileData

	// Regex pattern for your single-line delimiter format
	// Pattern: ---FILE-START filename="filename.ext"---\ncontent\n---FILE-END filename="filename.ext"---
	pattern := regexp.MustCompile(`---FILE-START filename="([^"]+)"---\s*\n(.*?)\n---FILE-END filename="[^"]+?"---`)

	matches := pattern.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			filename := match[1]
			content := strings.TrimSpace(match[2])

			// Validate that content looks like base64
			if fps.isValidBase64(content) {
				file := FileData{
					Filename: filename,
					Content:  content,
					MimeType: fps.getMimeType(filename),
					Size:     int64(len(content)),
				}
				files = append(files, file)
			}
		}
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no files found with single-line delimiter pattern")
	}

	return files, nil
}

// Fallback parsing for various delimiter patterns
func (fps *FileProcessingService) parseWithFallbackDelimiters(response string) ([]FileData, error) {
	var files []FileData

	// Try multiple delimiter patterns
	patterns := []string{
		// Alternative delimiter patterns
		`===FILE_START===\s*FILENAME:\s*([^\n]+)\s*CONTENT:\s*(.*?)\s*===FILE_END===`,
		`\*\*\*FILE:\s*([^\*]+)\*\*\*\s*(.*?)\s*\*\*\*END\*\*\*`,
		`FILE:\s*([^\n]+)\n(.*?)\nEND_FILE`,
		// More flexible pattern
		`(?i)file[_\s]*(?:start|begin)[^\n]*?([^\n\r]+)\n(.*?)(?i)(?:end|finish)`,
	}

	for _, patternStr := range patterns {
		pattern := regexp.MustCompile(patternStr)
		matches := pattern.FindAllStringSubmatch(response, -1)

		for _, match := range matches {
			if len(match) >= 3 {
				filename := strings.TrimSpace(match[1])
				content := strings.TrimSpace(match[2])

				if fps.isValidBase64(content) && filename != "" {
					file := FileData{
						Filename: filename,
						Content:  content,
						MimeType: fps.getMimeType(filename),
						Size:     int64(len(content)),
					}
					files = append(files, file)
				}
			}
		}

		if len(files) > 0 {
			return files, nil
		}
	}

	return nil, fmt.Errorf("no files found with fallback delimiter patterns")
}

// Last resort: extract any base64-like content and try to determine filenames
func (fps *FileProcessingService) parseWithContentExtraction(response string) ([]FileData, error) {
	var files []FileData

	// Look for base64-like content blocks
	base64Pattern := regexp.MustCompile(`([A-Za-z0-9+/]{20,}={0,2})`)
	base64Matches := base64Pattern.FindAllStringSubmatch(response, -1)

	// Look for potential filenames
	filenamePattern := regexp.MustCompile(`([a-zA-Z0-9_\-\.]+\.[a-zA-Z]{2,4})`)
	filenameMatches := filenamePattern.FindAllStringSubmatch(response, -1)

	// Try to pair filenames with base64 content
	for i, base64Match := range base64Matches {
		content := base64Match[1]
		if fps.isValidBase64(content) {
			filename := "extracted_file.txt" // default filename

			// Try to find a corresponding filename
			if i < len(filenameMatches) {
				filename = filenameMatches[i][1]
			}

			file := FileData{
				Filename: filename,
				Content:  content,
				MimeType: fps.getMimeType(filename),
				Size:     int64(len(content)),
			}
			files = append(files, file)
		}
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no extractable content found")
	}

	return files, nil
}

// Validate if a string is valid base64
func (fps *FileProcessingService) isValidBase64(s string) bool {
	// Basic validation: check length and character set
	if len(s) < 4 || len(s)%4 != 0 {
		return false
	}

	// Try to decode to validate
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil
}

// Read file and convert to FileData structure (unchanged)
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
	// Always encode content to base64
	encodedContent := base64.StdEncoding.EncodeToString(content)

	return &FileData{
		Filename: filepath.Base(filePath),
		Content:  encodedContent,
		MimeType: mimeType,
		Size:     stat.Size(),
	}, nil
}

// Save processed files to disk (unchanged)
func (fps *FileProcessingService) SaveOutputFiles(response *FileProcessingResponse, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	for _, fileData := range response.OutputFiles {
		filePath := filepath.Join(outputDir, fileData.Filename)

		// Always decode the base64 encoded content
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

// Helper function to determine MIME type (unchanged)
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

// Example usage with delimiter-based approach
func main() {
	apiKey := os.Getenv("PPLX_API_KEY")
	if apiKey == "" {
		fmt.Println("Please set PPLX_API_KEY environment variable")
		return
	}

	service := NewFileProcessingService(apiKey)

	// Example: Process text files using delimiter-based output
	fmt.Println("=== Processing Text Files with Delimiter-Based Output ===")
	inputPaths := []string{"input1.md", "input2.md"}
	var inFileData []FileData

	// Read input files
	for _, filePath := range inputPaths {
		data, err := service.readFile(filePath)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", filePath, err)
			return
		}
		inFileData = append(inFileData, *data)
	}

	// Specify output file details
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
			fmt.Println("✓ Output files saved to ./output directory")
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
