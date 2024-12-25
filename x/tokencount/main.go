package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/core"
)

func main() {
	// Inventory tree content using `ohcount -d <directory>`
	directory := "."
	if len(os.Args) > 1 {
		directory = os.Args[1]
	}

	fileLangMap, err := getFileLanguages(directory)
	if err != nil {
		log.Fatalf("Error getting file languages: %v", err)
	}

	langCounts := make(map[string]int)
	langFileCounts := make(map[string]int)
	totalTokens := 0
	totalFiles := 0

	// initialize Tokenizer
	var grok *core.Grokker
	err = core.InitTokenizer()
	Ck(err)

	// Count tokens and files in each language
	for file, lang := range fileLangMap {
		tokens, err := countTokens(grok, file)
		if err != nil {
			log.Printf("Failed to count tokens in %s: %v", file, err)
			continue
		}
		langCounts[lang] += tokens
		langFileCounts[lang]++
		totalTokens += tokens
		totalFiles++
	}

	// Prepare a slice for sorting
	type langStat struct {
		Language string
		Files    int
		Tokens   int
	}

	var stats []langStat
	for lang, count := range langCounts {
		stats = append(stats, langStat{
			Language: lang,
			Files:    langFileCounts[lang],
			Tokens:   count,
		})
	}

	// Sort by token count, highest first
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Tokens > stats[j].Tokens
	})

	// Report totals by language in columnar format
	fmt.Printf("%-16s  %-5s  %-9s\n", "Language", "Files", "Tokens")
	fmt.Printf("----------------  -----  ---------\n")
	for _, stat := range stats {
		fmt.Printf("%-16s  %5d  %9d\n", stat.Language, stat.Files, stat.Tokens)
	}
	fmt.Printf("----------------  -----  ---------\n")
	fmt.Printf("%-16s  %5d  %9d\n", "Total", totalFiles, totalTokens)
}

// getFileLanguages runs `ohcount -d <root>` and parses its output to map files to languages
func getFileLanguages(root string) (map[string]string, error) {
	cmd := exec.Command("ohcount", "-d", root)
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
		absolutePath, err := filepath.Abs(filepath.Join(root, filePath))
		if err != nil {
			log.Printf("Failed to get absolute path for %s: %v", filePath, err)
			continue
		}
		if lang == "(null)" {
			lang = "other"
		}
		fileLangMap[absolutePath] = lang
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading ohcount output: %v", err)
	}

	return fileLangMap, nil
}

// countTokens counts the number of tokens (words) in a file
func countTokens(grok *core.Grokker, file string) (count int, err error) {
	// Open the file
	f, err := os.Open(file)
	if err != nil {
		return 0, fmt.Errorf("failed to open file %s: %v", file, err)
	}
	defer f.Close()

	// Read the file content
	buf, err := ioutil.ReadAll(f)
	if err != nil {
		return 0, fmt.Errorf("failed to read file %s: %v", file, err)
	}
	content := string(buf)
	content = strings.TrimSpace(content)

	count, err = grok.TokenCount(content)

	return
}
