package main

import (
	"crypto/sha256"
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
- open a directory and find all files (each file’s name is its fitness score)
- for N generations:
	- for each file that does not have a numeric filename (i.e. unscored):
			- calculate its fitness by comparing it with a randomly chosen scored file (or another unscored file if none exists)
			- if the computed fitness score is already used by another file, increment the score until it is unique
			- rename the file with its unique numeric fitness score, preserving its extension
	- sort the files by fitness score; if a filename is non-numeric, sort alphanumerically instead of numerically
	- remove the files with the lowest fitness scores to reduce the population to half the desired size
		- do not remove files that are unscored
	- repopulate the directory by recombining the remaining files using the following procedure:
		- pick two different files at random (parents)
		- provide the two files to an LLM, returning a new file (child)
		- immediately compute the fitness of the child relative to one parent and rename both files with their new unique scores
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

	// directory containing individuals (files are named by their fitness scores)
	dir := flag.String("d", "", "Directory containing individuals")

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
	individuals, err := allIndividuals(dir)
	Ck(err)
	sortedFiles := sortIndividuals(individuals)
	for _, file := range sortedFiles {
		score, ok := parseFitnessFromFilename(file)
		if ok {
			fmt.Printf("%d %s\n", score, file)
		} else {
			fmt.Printf("%s\n", file)
		}
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

// parseFitnessFromFilename attempts to parse the score from a file's name.
// It strips the extension and tries to convert the rest to an int.
func parseFitnessFromFilename(filename string) (int, bool) {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	score, err := strconv.Atoi(base)
	if err != nil {
		return 0, false
	}
	return score, true
}

// updateFitness renames the given file to have the provided newScore as its base name,
// ensuring that no two files have the same score. It returns the new filename.
func updateFitness(file string, newScore int) (string, error) {
	ext := filepath.Ext(file)
	candidate := strconv.Itoa(newScore) + ext
	for {
		if candidate != file {
			if _, err := os.Stat(candidate); err == nil {
				newScore++
				candidate = strconv.Itoa(newScore) + ext
				continue
			}
		}
		break
	}
	if file != candidate {
		err := os.Rename(file, candidate)
		if err != nil {
			return "", err
		}
		Pf("Renamed %s to %s\n", file, candidate)
	}
	return candidate, nil
}

// sortIndividuals sorts a list of filenames. If both filenames have numeric bases,
// they are sorted in descending numerical order; otherwise, they are sorted in descending
// lexicographical order.
func sortIndividuals(files []string) []string {
	sort.Slice(files, func(i, j int) bool {
		scoreI, ok1 := parseFitnessFromFilename(files[i])
		scoreJ, ok2 := parseFitnessFromFilename(files[j])
		if ok1 && ok2 {
			return scoreI > scoreJ
		}
		return files[i] > files[j]
	})
	return files
}

// generation runs a single generation of the genetic algorithm
func generation(g *core.Grokker, gen int, dir string, fitnessCriteria string, model *core.Model, populationSize int) {
	defer Return(nil)

	Pf("Generation %d\n", gen)

	// Get all individuals (files) in the directory
	individuals, err := allIndividuals(dir)
	Ck(err)
	Assert(len(individuals) > 1, "Need at least two individuals to breed")

	scored := make(map[string]bool)
	unscored := make(map[string]bool)
	for _, file := range individuals {
		if _, ok := parseFitnessFromFilename(file); ok {
			scored[file] = true
		} else {
			unscored[file] = true
		}
	}

	// Process unscored files by comparing each to a scored file if available;
	// otherwise, pair up unscored files.
	for len(unscored) > 0 {
		// pick a random unscored file
		file1 := randMapKey(unscored)
		// pick a reference file
		ref := ""
		if len(scored) > 0 {
			// pick a random scored file
			ref = randMapKey(scored)
		} else {
			// pick a random unscored file
			ref = randMapKey(unscored)
		}
		// if the reference file is the same as the unscored file, pick another
		if file1 == ref {
			continue
		}

		// calculate the fitness of the unscored file relative to the reference file
		score1, score2, err := fitness(g, file1, ref, fitnessCriteria, model)
		if err != nil || score1 <= 0 || score2 <= 0 {
			log.Printf("Error calculating fitness for files %s and %s: scores %d and %d, %v", file1, ref, score1, score2, err)
			continue
		}

		// update the unscored file with its new fitness score
		newName, err := updateFitness(file1, score1)
		Ck(err)
		// update the reference file with its new fitness score
		newRefName, err := updateFitness(ref, score2)
		Ck(err)

		// update the maps
		delete(unscored, file1)
		delete(unscored, ref)
		delete(scored, file1)
		delete(scored, ref)
		scored[newName] = true
		scored[newRefName] = true

	}
	// Refresh the list of individuals after scoring
	individuals, err = allIndividuals(dir)
	Ck(err)
	sortedFiles := sortIndividuals(individuals)

	// Remove the files with the lowest fitness scores so that only the top half remain.
	if len(sortedFiles) > populationSize/2 {
		for i := populationSize / 2; i < len(sortedFiles); i++ {
			err = os.Remove(sortedFiles[i])
			Ck(err)
		}
	}

	// Repopulate the directory by recombining the remaining files until populationSize is reached.
	for {
		survivors, err := allIndividuals(dir)
		Ck(err)
		if len(survivors) >= populationSize {
			break
		}
		// pick two random survivors
		if len(survivors) < 2 {
			break
		}
		mom := survivors[rand.Intn(len(survivors))]
		dad := survivors[rand.Intn(len(survivors))]
		if mom == dad {
			continue
		}
		// Merge the two files. The merge function now returns the child's filename.
		child, err := merge(g, mom, dad, fitnessCriteria, model)
		Ck(err)
		// Always calculate the fitness of a child after generating it.
		// Calculate the fitness of the new child relative to one parent (mom).
		scoreMom, scoreChild, err := fitness(g, mom, child, fitnessCriteria, model)
		if err != nil || scoreMom <= 0 || scoreChild <= 0 {
			log.Printf("Error calculating fitness for files %s and %s: scores %d and %d, %v", mom, child, scoreMom, scoreChild, err)
			continue
		}
		newMom, err := updateFitness(mom, scoreMom)
		Ck(err)
		newChild, err := updateFitness(child, scoreChild)
		Ck(err)
		_ = newMom
		_ = newChild
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
	if err != nil {
		return 0, 0, fmt.Errorf("error calculating fitness: %v", err)
	}

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

// merge combines two files using the LLM.
// It returns the filename of the newly created child file.
func merge(g *core.Grokker, mom, dad string, fitnessCriteria string, model *core.Model) (child string, err error) {
	defer Return(&err)

	Pl()
	Pf("Merging %s and %s...\n", mom, dad)

	// create a new file name for the child using a hash of the parents
	child = childNameHash(mom, dad)

	// if the child file already exists, skip merge
	_, err = os.Stat(child)
	if err == nil {
		Pf("Child file %s already exists, skipping merge\n", child)
		return child, nil
	}
	Pf("Creating child file %s\n", child)

	ext := filepath.Ext(mom)
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
		return "", fmt.Errorf("error merging files: %v", err)
	}

	elapsed := time.Since(start)
	stopDots <- true
	close(stopDots)
	Pf(" got response in %s\n", elapsed)

	err = core.ExtractFiles(outFls, res, false, false)
	if err != nil {
		return "", fmt.Errorf("error extracting merged output: %v", err)
	}

	Pl()

	return child, nil
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

// randMapKey returns a random key from the map without having to
// first build a slice of keys
func randMapKey(m map[string]bool) string {
	Assert(len(m) > 0, "randMapKey: map is empty")
	j := rand.Intn(len(m))
	i := 0
	for key := range m {
		if i == j {
			return key
		}
		i++
	}
	Assert(false, "randMapKey: no key found")
	return "" // should never happen
}
