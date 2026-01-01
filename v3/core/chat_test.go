package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractFilesBasic(t *testing.T) {
	// get current directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	// create a temporary directory for output files
	tmpDir, err := os.MkdirTemp("", "extract_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	// defer os.RemoveAll(tmpDir)
	fmt.Println("Temp dir:", tmpDir)

	// cd to the temp directory
	err = os.Chdir(tmpDir)
	if err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	defer os.Chdir(cwd)

	tests := []struct {
		name             string
		testdataFile     string
		outfiles         []string
		expectExtracted  []string
		expectMissing    []string
		expectUnexpected int
		expectBroken     []string
		dryRun           bool
	}{
		{
			name:             "single_file_complete",
			testdataFile:     "single_file_complete.txt",
			outfiles:         []string{"output.txt"},
			expectExtracted:  []string{"output.txt"},
			expectMissing:    []string{},
			expectUnexpected: 0,
			expectBroken:     []string{},
			dryRun:           false,
		},
		{
			name:             "multiple_files_sequential",
			testdataFile:     "multiple_files_sequential.txt",
			outfiles:         []string{"file1.go", "file2.go", "file3.go"},
			expectExtracted:  []string{"file1.go", "file2.go", "file3.go"},
			expectMissing:    []string{},
			expectUnexpected: 0,
			expectBroken:     []string{},
			dryRun:           false,
		},
		{
			name:             "file_with_code",
			testdataFile:     "file_with_code.txt",
			outfiles:         []string{"main.go"},
			expectExtracted:  []string{"main.go"},
			expectMissing:    []string{},
			expectUnexpected: 0,
			expectBroken:     []string{},
			dryRun:           false,
		},
		{
			name:             "response_missing_file",
			testdataFile:     "response_missing_file.txt",
			outfiles:         []string{"expected.txt", "provided.txt"},
			expectExtracted:  []string{"provided.txt"},
			expectMissing:    []string{"expected.txt"},
			expectUnexpected: 0,
			expectBroken:     []string{},
			dryRun:           false,
		},
		{
			name:             "response_with_extra_file",
			testdataFile:     "response_with_extra_file.txt",
			outfiles:         []string{"expected.txt"},
			expectExtracted:  []string{"expected.txt"},
			expectMissing:    []string{},
			expectUnexpected: 1,
			expectBroken:     []string{},
			dryRun:           false,
		},
		{
			name:             "response_mixed_expected_unexpected",
			testdataFile:     "response_mixed_expected_unexpected.txt",
			outfiles:         []string{"file1.txt", "file2.txt", "file3.txt"},
			expectExtracted:  []string{"file1.txt", "file2.txt", "file3.txt"},
			expectMissing:    []string{},
			expectUnexpected: 2,
			expectBroken:     []string{},
			dryRun:           false,
		},
		{
			name:             "file_missing_end_marker",
			testdataFile:     "file_missing_end_marker.txt",
			outfiles:         []string{"incomplete.txt"},
			expectExtracted:  []string{},
			expectMissing:    []string{"incomplete.txt"},
			expectUnexpected: 0,
			expectBroken:     []string{"incomplete.txt"},
			dryRun:           true,
		},
		{
			name:             "mismatched_file_markers",
			testdataFile:     "mismatched_file_markers.txt",
			outfiles:         []string{"file1.txt"},
			expectExtracted:  []string{},
			expectMissing:    []string{"file1.txt"},
			expectUnexpected: 0,
			expectBroken:     []string{"file1.txt", "file2.txt"},
			dryRun:           true,
		},
		{
			name:             "end_marker_without_start",
			testdataFile:     "end_marker_without_start.txt",
			outfiles:         []string{},
			expectExtracted:  []string{},
			expectMissing:    []string{},
			expectUnexpected: 0,
			expectBroken:     []string{"orphaned.txt"},
			dryRun:           true,
		},
		{
			name:             "nested_files",
			testdataFile:     "nested_files.txt",
			outfiles:         []string{"outer.txt", "inner.txt"},
			expectExtracted:  []string{"outer.txt", "inner.txt"},
			expectMissing:    []string{},
			expectUnexpected: 0,
			expectBroken:     []string{},
			dryRun:           true,
		},
		{
			name:             "special_char_filenames",
			testdataFile:     "special_char_filenames.txt",
			outfiles:         []string{"my-file.json", "src/main.go", "config_prod.yaml"},
			expectExtracted:  []string{"my-file.json", "src/main.go", "config_prod.yaml"},
			expectMissing:    []string{},
			expectUnexpected: 0,
			expectBroken:     []string{},
			dryRun:           true,
		},
		{
			name:             "file_content_with_marker_text",
			testdataFile:     "file_content_with_marker_text.txt",
			outfiles:         []string{"readme.md"},
			expectExtracted:  []string{"readme.md"},
			expectMissing:    []string{},
			expectUnexpected: 1,
			expectBroken:     []string{},
			dryRun:           true,
		},
		{
			name:             "empty_file",
			testdataFile:     "empty_file.txt",
			outfiles:         []string{"empty.txt"},
			expectExtracted:  []string{"empty.txt"},
			expectMissing:    []string{},
			expectUnexpected: 0,
			expectBroken:     []string{},
			dryRun:           true,
		},
		{
			name:             "response_with_thinking",
			testdataFile:     "response_with_thinking.txt",
			outfiles:         []string{"output.txt"},
			expectExtracted:  []string{"output.txt"},
			expectMissing:    []string{},
			expectUnexpected: 0,
			expectBroken:     []string{},
			dryRun:           true,
		},
		{
			name:             "response_with_references",
			testdataFile:     "response_with_references.txt",
			outfiles:         []string{"report.txt"},
			expectExtracted:  []string{"report.txt"},
			expectMissing:    []string{},
			expectUnexpected: 0,
			expectBroken:     []string{},
			dryRun:           true,
		},
		{
			name:             "response_with_reasoning",
			testdataFile:     "response_with_reasoning.txt",
			outfiles:         []string{"analysis.txt"},
			expectExtracted:  []string{"analysis.txt"},
			expectMissing:    []string{},
			expectUnexpected: 0,
			expectBroken:     []string{},
			dryRun:           true,
		},
		{
			name:             "complete_response_with_metadata",
			testdataFile:     "complete_response_with_metadata.txt",
			outfiles:         []string{"result.txt"},
			expectExtracted:  []string{"result.txt"},
			expectMissing:    []string{},
			expectUnexpected: 0,
			expectBroken:     []string{},
			dryRun:           true,
		},
		{
			name:             "response_for_dryrun",
			testdataFile:     "response_for_dryrun.txt",
			outfiles:         []string{"file1.txt", "file2.txt"},
			expectExtracted:  []string{"file1.txt", "file2.txt"},
			expectMissing:    []string{},
			expectUnexpected: 0,
			expectBroken:     []string{},
			dryRun:           true,
		},
		{
			name:             "response_for_stdout",
			testdataFile:     "response_for_stdout.txt",
			outfiles:         []string{"stdout.txt"},
			expectExtracted:  []string{"stdout.txt"},
			expectMissing:    []string{},
			expectUnexpected: 0,
			expectBroken:     []string{},
			dryRun:           true,
		},
		{
			name:             "complex_response_for_dryrun_metadata",
			testdataFile:     "complex_response_for_dryrun_metadata.txt",
			outfiles:         []string{"expected1.txt", "expected2.txt"},
			expectExtracted:  []string{"expected1.txt"},
			expectMissing:    []string{"expected2.txt"},
			expectUnexpected: 1,
			expectBroken:     []string{"broken.txt"},
			dryRun:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read testdata file
			infn := filepath.Join(cwd, "testdata", "extract_files", tt.testdataFile)
			data, err := os.ReadFile(infn)
			if err != nil {
				t.Fatalf("Failed to read testdata file %s: %v", infn, err)
			}

			// Run ExtractFiles
			result, err := ExtractFiles(tt.outfiles, string(data), ExtractOptions{
				DryRun:          tt.dryRun,
				ExtractToStdout: false,
			})
			if err != nil {
				t.Fatalf("ExtractFiles failed: %v", err)
			}

			// Verify extracted files
			if len(result.ExtractedFiles) != len(tt.expectExtracted) {
				t.Errorf("Expected %d extracted files, got %d: %v", len(tt.expectExtracted), len(result.ExtractedFiles), result.ExtractedFiles)
			}

			// Verify missing files
			if len(result.MissingFiles) != len(tt.expectMissing) {
				t.Errorf("Expected %d missing files, got %d: %v", len(tt.expectMissing), len(result.MissingFiles), result.MissingFiles)
			}

			// Verify unexpected files
			if len(result.UnexpectedFiles) != tt.expectUnexpected {
				t.Errorf("Expected %d unexpected files, got %d", tt.expectUnexpected, len(result.UnexpectedFiles))
			}

			// Verify broken files
			if len(result.BrokenFiles) != len(tt.expectBroken) {
				t.Errorf("Expected %d broken files, got %d: %v", len(tt.expectBroken), len(result.BrokenFiles), result.BrokenFiles)
			}

			// Verify raw response is unchanged
			if result.RawResponse != string(data) {
				t.Error("RawResponse was modified")
			}

			// Verify cooked response is not empty (unless all content was files)
			if tt.name != "single_file_complete" && len(result.CookedResponse) == 0 {
				t.Error("CookedResponse should contain non-file content")
			}
		})
	}
}

func TestExtractFilesFileWriting(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "extract_test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	// defer os.RemoveAll(tmpDir)
	fmt.Println("Temp dir:", tmpDir)

	outputFile := filepath.Join(tmpDir, "output.txt")
	responseContent := "Here is the output file:\n\n---FILE-START filename=\"" + outputFile + "\"---\nTest content\n---FILE-END filename=\"" + outputFile + "\"---\n\nDone."

	result, err := ExtractFiles(
		[]string{outputFile},
		responseContent,
		ExtractOptions{DryRun: false, ExtractToStdout: false},
	)

	if err != nil {
		t.Fatalf("ExtractFiles failed: %v", err)
	}

	if len(result.ExtractedFiles) != 1 || result.ExtractedFiles[0] != outputFile {
		t.Errorf("Expected file %s to be extracted", outputFile)
	}

	// Verify file was actually written
	data, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}

	if string(data) != "Test content" {
		t.Errorf("Expected 'Test content', got '%s'", string(data))
	}
}
