package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/v3/client"
	"github.com/stevegt/grokker/v3/core"
)

/*

This program evolves a set of files using a genetic algorithm using the following procedure:

- read a file containing fitness criteria
- open a directory and find all files
- for N generations:
	- for each file:
		- check the .scores file in the same directory for a fitness score
		- if there is no score found:
			- provide the file and the fitness criteria to an LLM, returning a fitness score
			- add the fitness score to the .scores file
	- sort the files by fitness score
	- remove the files with the lowest fitness scores and remove their entries from the .scores file
	- repopulate the directory by recombining the remaining files using the following procedure:
		- pick two files
		- provide the two files to an LLM, returning a new file
- git commit

*/

func usage(models *core.Models) {
	fmt.Println("Usage: go run main.go -f <fitnessFn> -d <dir> -m <model> -g <generations> -p <populationSize>")
	// list models
	fmt.Println("Available models:")
	for _, m := range models.ListModels() {
		fmt.Printf("  %s\n", m.String())
	}
}

var commitPrompt = "Write a git commit message summarizing the following file. Use present tense, active, imperative statements as if giving directions.  Do not use extra adjectives or marketing hype.  The first line of the commit message must be a summary of 60 characters or less, followed by a blank line, followed by bullet-pointed details."

var mergePromptTmpl = "Create a new file combining the best parts of the following two files while improving the result to meet the fitness criteria specified below.  The new file must be a valid file of the same type as the first two files.  The fitness criteria are:\n\n%s\n\n"

var fitnessPromptTmpl = `Provide fitness scores for the two files. The first file is delimited by <file1> and the second file is delimited by <file2>. Include the fitness criteria found between <fitnessCriteria> tags below. Your response must include two scores in the format 'fitness1=<number>' and 'fitness2=<number>'. Ensure that the scores are not equal, both are greater than zero, and that the higher score corresponds to the file that better meets the fitness criteria.`

func main() {
	// Define and parse command-line flags

	// file that contains fitness criteria
	fitnessFn := flag.String("f", "", "File containing fitness criteria")

	// directory containing individuals and .scores file
	dir := flag.String("d", "", "Directory containing individuals and .scores file")

	// LLM model
	modelName := flag.String("m", "", "LLM model")

	// number of generations
	generations := flag.Int("g", 0, "Number of generations")

	// population size
	populationSize := flag.Int("p", 0, "Population size")

	// parse the flags
	flag.Parse()

	models := core.NewModels()

	// if any flag is missing, print usage and exit
	if *fitnessFn == "" || *dir == "" || *modelName == "" || *generations == 0 || *populationSize == 0 {
		usage(models)
		os.Exit(1)
	}

	// Check if the specified model is available
	model, ok := models.Available[*modelName]
	if !ok {
		usage(models)
		log.Fatalf("Model %s is not available", *modelName)
	}

	// Read the fitness criteria from the file
	fitnessCriteriaBuf, err := ioutil.ReadFile(*fitnessFn)
	Ck(err)
	fitnessCriteria := string(fitnessCriteriaBuf)

	// change to the specified directory
	err = os.Chdir(*dir)
	Ck(err)

	// initialize grokker
	err = core.InitTokenizer()
	Ck(err)
	g := &core.Grokker{
		Root:    *dir,
		Version: core.Version,
	}
	err = g.Setup(*modelName)
	Ck(err)

	for i := 0; i < *generations; i++ {
		generation(g, i, *dir, fitnessCriteria, model, *populationSize)
		// print the name and fitness score of each file
		showAll(*dir)
	}

	/*
		// generate a commit message using the content of the fittest file
		// and the commitPrompt
		// XXX

		// git commit the changes
		cmd := exec.Command("git", "add", *dir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		Ck(err)
	*/

}

// showAll prints the name and fitness score of each file
func showAll(dir string) {
	scoresFn := dir + "/.scores"
	scores := &Scores{}
	// read the .scores file
	scoresBytes, err := ioutil.ReadFile(scoresFn)
	Ck(err)
	// unmarshal the .scores file
	err = json.Unmarshal(scoresBytes, scores)
	Ck(err)

	// sort the scores by fitness score
	sortedScores := make([]string, 0, len(scores.Fitness))
	for file := range scores.Fitness {
		sortedScores = append(sortedScores, file)
	}
	sort.Slice(sortedScores, func(i, j int) bool {
		return scores.Fitness[sortedScores[i]] > scores.Fitness[sortedScores[j]]
	})

	for _, file := range sortedScores {
		score := scores.Fitness[file]
		fmt.Printf("%d %s\n", score, file)
	}
}

// allIndividuals returns a list of all individuals in the directory
func allIndividuals(dir string) ([]string, error) {
	files, err := ioutil.ReadDir(".")
	if err != nil {
		return nil, err
	}
	individuals := make([]string, 0)
	for _, file := range files {
		if strings.HasPrefix(file.Name(), ".") {
			continue
		}
		if file.IsDir() {
			continue
		}
		individuals = append(individuals, file.Name())
	}
	return individuals, nil
}

// Scores is a map that holds the fitness scores for each file
type Scores struct {
	Fitness map[string]int `json:"fitness"`
}

// generation runs a single generation of the genetic algorithm
func generation(g *core.Grokker, gen int, dir string, fitnessCriteria string, model *core.Model, populationSize int) {
	defer Return(nil)

	Pf("Generation %d\n", gen)

	// Open the directory and find all files
	files, err := allIndividuals(dir)
	Ck(err)

	// read the .scores file
	scoresFn := dir + "/.scores"
	scores := &Scores{}
	// unmarshal or create the .scores file
	for {
		// see if the .scores file exists
		if _, err := os.Stat(scoresFn); os.IsNotExist(err) {
			// create the .scores file if it doesn't exist
			scores.Fitness = make(map[string]int)
			scoresBytes, err := json.Marshal(scores)
			Ck(err)
			err = ioutil.WriteFile(scoresFn, scoresBytes, 0644)
			Ck(err)
		}
		// read the .scores file
		scoresBytes, err := ioutil.ReadFile(scoresFn)
		Ck(err)
		// unmarshal the .scores file
		err = json.Unmarshal(scoresBytes, scores)
		if err != nil {
			// bad .scores file, delete it and try again
			err = os.Remove(scoresFn)
			Ck(err)
			continue
		}
		// good .scores file, break out of the loop
		break
	}

	// Evaluate unscored files
	unscored := []string{}
	scored := []string{}
	for _, file := range files {
		if _, ok := scores.Fitness[file]; !ok {
			unscored = append(unscored, file)
		} else {
			scored = append(scored, file)
		}
	}

	// Process unscored files, comparing each to a randomly selected
	// scored file
	for i := 0; i+1 < len(unscored); i += 2 {
		file1 := unscored[i]
		file2 := scored[rand.Intn(len(scored))]
		score1, score2, err := fitness(g, file1, file2, fitnessCriteria, model)
		if err != nil || score1 <= 0 || score2 <= 0 {
			log.Printf("Error calculating fitness for files %s and %s: scores %d and %d, %v", file1, file2, score1, score2, err)
			continue
		}
		scores.Fitness[file1] = score1
		scores.Fitness[file2] = score2

		// write the fitness scores to the .scores file after each pair
		scoresBytes, err := json.Marshal(scores)
		Ck(err)
		err = ioutil.WriteFile(scoresFn, scoresBytes, 0644)
		Ck(err)
	}

	// sort the files by fitness score
	sortedFiles := make([]string, 0, len(scores.Fitness))
	for file := range scores.Fitness {
		sortedFiles = append(sortedFiles, file)
	}
	sort.Slice(sortedFiles, func(i, j int) bool {
		// sort in descending order
		return scores.Fitness[sortedFiles[i]] > scores.Fitness[sortedFiles[j]]
	})

	// remove the files with the lowest fitness scores
	for i := len(sortedFiles) - 1; i >= populationSize/2; i-- {
		// remove the file from the scores map
		delete(scores.Fitness, sortedFiles[i])
		// remove the file from the directory
		err = os.Remove(dir + "/" + sortedFiles[i])
		if err != nil {
			log.Printf("Error removing file %s: %v", sortedFiles[i], err)
			continue
		}
	}
	// write the scores map to the .scores file
	scoresBytes, err := json.Marshal(scores)
	Ck(err)
	err = ioutil.WriteFile(scoresFn, scoresBytes, 0644)
	Ck(err)

	// repopulate the directory by recombining the remaining files
	for {
		// count the number of files in the directory
		survivors, err := allIndividuals(dir)
		Ck(err)
		if len(survivors) >= populationSize {
			break
		}

		// pick two files at random
		mom := survivors[rand.Intn(len(survivors))]
		dad := survivors[rand.Intn(len(survivors))]

		// check if the files are the same
		if mom == dad {
			continue
		}

		// provide the two files to an LLM, creating a new file
		err = merge(g, mom, dad, fitnessCriteria, model)
		Ck(err)
	}

	return
}

// fitness calculates the fitness scores for two files
// It returns two scores (score1 and score2) corresponding to file1 and file2 respectively.
func fitness(g *core.Grokker, file1, file2 string, fitnessCriteria string, model *core.Model) (int, int, error) {
	defer Return(nil)

	Pf("Calculating fitness for %s and %s ...\n", file1, file2)

	// get the contents of both files
	buf1, err := ioutil.ReadFile(file1)
	Ck(err)
	txt1 := string(buf1)

	buf2, err := ioutil.ReadFile(file2)
	Ck(err)
	txt2 := string(buf2)

	// create the fitness prompt by wrapping the files and fitness criteria in XML tags
	fitnessPrompt := fmt.Sprintf(fitnessPromptTmpl)
	prompt := fmt.Sprintf("%s\n\n<fitnessCriteria>%s</fitnessCriteria>\n\n<file1>%s</file1>\n\n<file2>%s</file2>",
		fitnessPrompt, fitnessCriteria, txt1, txt2)

	// call the LLM with the fitness prompt
	msgs := []client.ChatMsg{
		{
			Role:    "User",
			Content: prompt,
		},
	}
	res, err := g.CompleteChat(model.Name, fitnessPrompt, msgs)
	Ck(err)

	// find fitness1 and fitness2 in the response
	re1 := regexp.MustCompile(`fitness1=(\d+)`)
	m1 := re1.FindStringSubmatch(res)
	if len(m1) < 2 {
		return 0, 0, fmt.Errorf("fitness1 not found in response: %s", res)
	}
	re2 := regexp.MustCompile(`fitness2=(\d+)`)
	m2 := re2.FindStringSubmatch(res)
	if len(m2) < 2 {
		return 0, 0, fmt.Errorf("fitness2 not found in response: %s", res)
	}

	// convert the scores to ints
	score1, err := strconv.Atoi(m1[1])
	if err != nil {
		return 0, 0, err
	}
	score2, err := strconv.Atoi(m2[1])
	if err != nil {
		return 0, 0, err
	}

	// Ensure the scores are not equal; if they are, adjust slightly.
	if score1 == score2 {
		score2 = score1 + 1
	}

	Pf("    %s: %d\n", file1, score1)
	Pf("    %s: %d\n", file2, score2)

	return score1, score2, nil
}

// countTokens counts the number of tokens (words) in a file
func countTokens(g *core.Grokker, file string) (count int, err error) {
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

	count, err = g.TokenCount(content)
	return
}

// Prompt is a struct that represents a prompt
type Prompt struct {
	Sysmsg string
	In     []string
	Out    []string
	Txt    string
}

// childNameComma creates a new file name for the child by concatenating the two
// parents, splitting on commas, removing duplicates, then joining
// the parts with a comma and adding a random string and the extension of the first
// parent
func childNameComma(mom, dad string) string {
	ext := filepath.Ext(mom)
	momBase := strings.TrimSuffix(mom, ext)
	dadBase := strings.TrimSuffix(dad, ext)

	parts := strings.Split(momBase, ",")
	parts = append(parts, strings.Split(dadBase, ",")...)
	// remove duplicates
	partsMap := make(map[string]bool)
	for _, part := range parts {
		partsMap[part] = true
	}
	// create a new slice with the unique parts
	parts = make([]string, 0, len(partsMap))
	for part := range partsMap {
		parts = append(parts, part)
	}
	childBase := strings.Join(parts, ",")
	child := fmt.Sprintf("%s,%s%s", childBase, randString(4), ext)

	return child
}

// childNameHash creates a new file name for the child by concatenating the two
// parents, hashing the result, and adding the extension of the first parent
func childNameHash(mom, dad string) string {
	ext := filepath.Ext(mom)
	// hash the two parents
	hash := sha256.New()
	hash.Write([]byte(mom + dad))
	// get the digest
	digest := hash.Sum(nil)
	// convert the digest to a hex string and add the extension
	child := fmt.Sprintf("%x%s", digest, ext)
	return child
}

// merge combines two files using the LLM
func merge(g *core.Grokker, mom, dad string, fitnessCriteria string, model *core.Model) (err error) {
	defer Return(&err)

	Pl()
	Pf("Merging %s and %s...\n", mom, dad)

	// create a new file name for the child
	ext := filepath.Ext(mom)

	child := childNameHash(mom, dad)

	// if the child file already exists, return with no error
	_, err = os.Stat(child)
	if err == nil {
		Pf("Child file %s already exists, skipping merge\n", child)
		return nil
	}
	Pf("Creating child file %s: %v\n", child, err)

	extNoDot := strings.TrimPrefix(ext, ".")

	inFns := []string{mom, dad}
	outFls := []core.FileLang{
		{
			File:     child,
			Language: extNoDot,
		},
	}

	// Count tokens
	Pf("Token counts:\n")
	total := 0
	for _, f := range inFns {
		count, err := countTokens(g, f)
		Ck(err)
		Pf("    %s: %d\n", f, count)
		total += count
	}
	Pf("    Total: %d\n", total)

	Pl("Output files:")
	for _, f := range outFls {
		Pf("    %s\n", f.File)
	}

	Pf("Querying GPT...")
	// Start a goroutine to print dots while waiting for the response
	var stopDots = make(chan bool)
	go func() {
		for {
			select {
			case <-stopDots:
				return
			default:
				time.Sleep(1 * time.Second)
				fmt.Print(".")
			}
		}
	}()
	start := time.Now()

	mergePrompt := fmt.Sprintf(mergePromptTmpl, fitnessCriteria)

	msgs := []client.ChatMsg{
		{
			Role:    "User",
			Content: mergePrompt,
		},
	}

	res, err := g.SendWithFiles(model.Name, mergePrompt, msgs, inFns, outFls)
	if err != nil {
		return fmt.Errorf("error merging files: %v", err)
	}

	elapsed := time.Since(start)
	stopDots <- true
	close(stopDots)
	Pf(" got response in %s\n", elapsed)

	// ExtractFiles(outFls, promptFrag, dryrun, extractToStdout)
	err = core.ExtractFiles(outFls, res, false, false)
	if err != nil {
		return fmt.Errorf("error extracting merged output: %v", err)
	}

	Pl()

	return
}

// randString generates a random string of the specified length
func randString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
