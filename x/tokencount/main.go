package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	// Inventory tree content using `ohcount -d`
	fileLangMap, err := getFileLanguages(".")
	if err != nil {
		log.Fatalf("Error getting file languages: %v", err)
	}

	langCounts := make(map[string]int)
	totalTokens := 0

	// Count tokens in each file, totaling by language
	for file, lang := range fileLangMap {
		tokens, err := countTokens(file)
		if err != nil {
			log.Printf("Failed to count tokens in %s: %v", file, err)
			continue
		}
		langCounts[lang] += tokens
		totalTokens += tokens
	}

	// Report totals by language
	fmt.Println("Token counts by language:")
	for lang, count := range langCounts {
		fmt.Printf("%s: %d\n", lang, count)
	}

	// Report grand total
	fmt.Printf("Grand total: %d\n", totalTokens)
}

// getFileLanguages runs `ohcount -d` and parses its output to map files to languages
func getFileLanguages(root string) (map[string]string, error) {
	cmd := exec.Command("ohcount", "-d")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ohcount command failed: %v, output: %s", err, out.String())
	}

	fileLangMap := make(map[string]string)
	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		lang := parts[0]
		filePath := strings.Join(parts[1:], " ")
		absolutePath, err := filepath.Abs(filePath)
		if err != nil {
			log.Printf("Failed to get absolute path for %s: %v", filePath, err)
			continue
		}
		fileLangMap[absolutePath] = lang
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading ohcount output: %v", err)
	}

	return fileLangMap, nil
}

// countTokens counts the number of tokens (words) in a file
func countTokens(file string) (int, error) {
	f, err := os.Open(file)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanWords)
	count := 0
	for scanner.Scan() {
		count++
	}

	if err := scanner.Err(); err != nil {
		return count, err
	}

	return count, nil
}
