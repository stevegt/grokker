package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/core"
)

func main() {
	// Define and parse command-line flags
	showIndividual := flag.Bool("i", false, "Show individual token counts for each file")
	listFiles := flag.Bool("l", false, "List all countable files")
	flag.Parse()

	// Determine the directories to process
	directories := flag.Args()
	if len(directories) == 0 {
		directories = []string{"."}
	}

	fileLangMap := make(map[string]string)
	// Process each directory and merge file language mappings
	for _, dir := range directories {
		mapping, err := getFileLanguages(dir)
		if err != nil {
			log.Fatalf("Error getting file languages from directory %s: %v", dir, err)
		}
		for file, lang := range mapping {
			fileLangMap[file] = lang
		}
	}

	// If -l flag is set, list all countable files and exit
	if *listFiles {
		for file := range fileLangMap {
			fmt.Println(file)
		}
		os.Exit(0)
	}

	langCounts := make(map[string]int)
	langFileCounts := make(map[string]int)
	totalTokens := 0
	totalFiles := 0

	// Initialize Tokenizer
	var grok *core.Grokker
	err := core.InitTokenizer()
	Ck(err)

	// Prepare slice for individual file stats if -i flag is set
	type fileStat struct {
		Language string
		Tokens   int
		File     string
	}
	var individualStats []fileStat

	// Count tokens and files in each language
	for path, lang := range fileLangMap {
		tokens, err := countTokens(grok, path)
		if err != nil {
			log.Printf("Failed to count tokens in %s: %v", path, err)
			continue
		}
		langCounts[lang] += tokens
		langFileCounts[lang]++
		totalTokens += tokens
		totalFiles++

		individualStats = append(individualStats, fileStat{
			Language: lang,
			Tokens:   tokens,
			File:     path,
		})
	}

	if *showIndividual {
		// Sort individual stats by token count in descending order
		sort.Slice(individualStats, func(i, j int) bool {
			return individualStats[i].Tokens > individualStats[j].Tokens
		})

		// Print header
		fmt.Printf("%-20s  %-7s  %-50s\n", "Language", "Tokens", "File")
		fmt.Printf("%-20s  %-7s  %-50s\n", strings.Repeat("-", 20), strings.Repeat("-", 7), strings.Repeat("-", 50))

		// Print each file's stats
		for _, stat := range individualStats {
			fmt.Printf("%-20s  %7d  %-50s\n", stat.Language, stat.Tokens, stat.File)
		}

		// Print footer
		fmt.Printf("%-20s  %-7s  %-50s\n", strings.Repeat("-", 20), strings.Repeat("-", 7), strings.Repeat("-", 50))
		fmt.Printf("%-20s  %7d  %-50s\n", "Total", totalTokens, "All Files")
	} else {
		// Prepare a slice for sorting languages
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
}

// getFileLanguages runs ohcount -d <root> and parses its output to map files to languages
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
		if lang == "(null)" {
			mtype, err := mimetype.DetectFile(filePath)
			if err != nil {
				log.Printf("Failed to detect mimetype for %s: %v", filePath, err)
				continue
			}
			if mtype.Is("text/plain") {
				lang = "text/plain"
			} else {
				// Ignore non-text/plain files
				continue
			}
		}
		fileLangMap[filePath] = lang
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
