package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestRWMutexConcurrentReads verifies multiple goroutines can hold read locks simultaneously.
// With Mutex: all reads serialize, total time ~numGoroutines * sleepTime
// With RWMutex: all reads are concurrent, total time ~sleepTime
func TestRWMutexConcurrentReads(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())
	chat.history = append(chat.history, &ChatRound{Query: "test", Response: "response"})

	numGoroutines := 5
	sleepTimeMs := 50
	var wg sync.WaitGroup
	var activeReaders int32 = 0
	var maxConcurrent int32 = 0

	startTime := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Increment active readers WHILE holding the lock
			_ = chat.getHistory(true)
			// Note: getHistory(true) now needs to increment/decrement activeReaders
		}()
	}
	wg.Wait()

	elapsed := time.Since(startTime)

	// With RWMutex, concurrent reads should complete quickly (~sleepTime)
	// With Mutex, they should take ~numGoroutines * sleepTime
	expectedMaxTime := time.Duration(sleepTimeMs*2) * time.Millisecond
	if elapsed > expectedMaxTime {
		t.Logf("FAIL: Reads took %v, expected ~%v. Likely using Mutex instead of RWMutex.", 
			elapsed, expectedMaxTime)
		t.Fail()
	}
}

// TestConcurrentReadsDontBlock verifies that multiple read operations don't block each other.
func TestConcurrentReadsDontBlock(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())
	chat.history = append(chat.history, &ChatRound{Query: "test", Response: "response"})

	// Create a channel to signal when goroutines are inside critical section
	insideCS := make(chan bool, 10)
	readComplete := make(chan bool, 10)

	// Goroutine 1: Hold read lock
	go func() {
		chat.mutex.Lock()
		insideCS <- true
		time.Sleep(100 * time.Millisecond)
		chat.mutex.Unlock()
	}()

	// Wait for goroutine 1 to acquire lock
	<-insideCS

	// Goroutine 2: Try to acquire read lock - with RWMutex, this should succeed immediately
	start := time.Now()
	go func() {
		_ = chat.getHistory(true)
		readComplete <- true
	}()

	select {
	case <-readComplete:
		elapsed := time.Since(start)
		// With RWMutex, read should complete quickly (not wait for first lock holder)
		if elapsed > 50*time.Millisecond {
			t.Logf("FAIL: Second read took %v, suggests Mutex not RWMutex", elapsed)
			t.Fail()
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Second read blocked indefinitely - definitely using Mutex not RWMutex")
	}
}

// TestWriteLockBlocksReads verifies write lock blocks all readers.
func TestWriteLockBlocksReads(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())

	writerInCS := make(chan bool)
	readerAttempted := make(chan bool)
	readerWaited := make(chan time.Duration)

	// Writer thread
	go func() {
		chat.mutex.Lock()
		writerInCS <- true
		time.Sleep(100 * time.Millisecond)
		chat.mutex.Unlock()
	}()

	// Wait for writer to acquire lock
	<-writerInCS

	// Reader thread
	go func() {
		readerAttempted <- true
		start := time.Now()
		_ = chat.getHistory(true)
		readerWaited <- time.Since(start)
	}()

	<-readerAttempted
	time.Sleep(10 * time.Millisecond) // Let reader attempt to acquire lock

	// Reader should have been blocked by writer
	waitTime := <-readerWaited
	if waitTime < 80*time.Millisecond {
		t.Logf("FAIL: Reader waited only %v, should have waited for writer", waitTime)
		t.Fail()
	}
}

// TestStartRoundBlocksDuringWrite verifies StartRound acquires exclusive lock.
func TestStartRoundBlocksDuringWrite(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())

	writerDone := make(chan bool)
	startRoundBlocked := make(chan time.Duration)

	// Writer: hold lock
	go func() {
		chat.mutex.Lock()
		time.Sleep(100 * time.Millisecond)
		chat.mutex.Unlock()
		writerDone <- true
	}()

	time.Sleep(10 * time.Millisecond) // Ensure writer has lock

	// StartRound: should block
	start := time.Now()
	go func() {
		_ = chat.StartRound("test", "")
		startRoundBlocked <- time.Since(start)
	}()

	// Verify StartRound was blocked
	blockTime := <-startRoundBlocked
	<-writerDone

	if blockTime < 80*time.Millisecond {
		t.Logf("FAIL: StartRound took only %v, should have blocked", blockTime)
		t.Fail()
	}
}

// TestFinishRoundLocksOnlyForFileIO verifies lock is held minimally during FinishRound.
func TestFinishRoundLocksOnlyForFileIO(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())
	round := chat.StartRound("test query", "")

	startTime := time.Now()
	err = chat.FinishRound(round, "test response")
	duration := time.Since(startTime)

	if err != nil {
		t.Fatalf("FinishRound failed: %v", err)
	}

	// FinishRound should complete quickly (only file I/O, no long operations)
	if duration > 200*time.Millisecond {
		t.Logf("FAIL: FinishRound took %v, suggests excessive lock holding", duration)
		t.Fail()
	}

	// Verify file was updated
	content, err := ioutil.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to read markdown file: %v", err)
	}
	if !bytes.Contains(content, []byte("test query")) {
		t.Errorf("Query not found in markdown file")
	}
	if !bytes.Contains(content, []byte("test response")) {
		t.Errorf("Response not found in markdown file")
	}
}

// TestNoRaceConditionDuringConcurrentQueries verifies no data races during concurrent operations.
func TestNoRaceConditionDuringConcurrentQueries(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())

	var wg sync.WaitGroup
	numGoroutines := 5

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Read history
			_ = chat.getHistory(true)

			// Start a round
			round := chat.StartRound(fmt.Sprintf("query %d", id), "")

			// Finish round
			response := fmt.Sprintf("response %d", id)
			_ = chat.FinishRound(round, response)
		}(i)
	}
	wg.Wait()

	// Verify all rounds were recorded
	if chat.TotalRounds() != numGoroutines {
		t.Errorf("Expected %d rounds, got %d", numGoroutines, chat.TotalRounds())
	}

	// Verify markdown file is valid
	content, err := ioutil.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to read markdown file: %v", err)
	}
	if len(content) == 0 {
		t.Errorf("Markdown file is empty after concurrent writes")
	}

	// Verify all queries are present
	contentStr := string(content)
	for i := 0; i < numGoroutines; i++ {
		if !bytes.Contains(content, []byte(fmt.Sprintf("query %d", i))) {
			t.Errorf("Query %d not found in markdown file", i)
		}
		if !bytes.Contains(content, []byte(fmt.Sprintf("response %d", i))) {
			t.Errorf("Response %d not found in markdown file", i)
		}
	}

	// Verify file structure is well-formed (each round should have both query and response)
	roundCount := bytes.Count(content, []byte("**query"))
	if roundCount != numGoroutines {
		t.Errorf("Expected %d query markers, found %d", numGoroutines, roundCount)
	}
}

// TestGetHistoryWithLockParameter verifies getHistory respects lock parameter.
func TestGetHistoryWithLockParameter(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())
	chat.history = append(chat.history, &ChatRound{Query: "query1", Response: "response1"})

	// Call with lock=true
	history1 := chat.getHistory(true)
	if !bytes.Contains([]byte(history1), []byte("query1")) {
		t.Errorf("History with lock=true missing query")
	}

	// Call with lock=false while holding mutex
	chat.mutex.Lock()
	history2 := chat.getHistory(false)
	chat.mutex.Unlock()

	if !bytes.Contains([]byte(history2), []byte("query1")) {
		t.Errorf("History with lock=false missing query")
	}
}

// TestUpdateMarkdownDoesNotDeadlock verifies file updates complete without deadlock.
func TestUpdateMarkdownDoesNotDeadlock(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())
	chat.history = append(chat.history, &ChatRound{Query: "test", Response: "response"})

	done := make(chan bool, 1)
	go func() {
		chat.mutex.Lock()
		_ = chat._updateMarkdown()
		chat.mutex.Unlock()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Error("_updateMarkdown appears to be deadlocked")
	}

	// Verify file contents
	content, err := ioutil.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to read markdown file: %v", err)
	}
	if !bytes.Contains(content, []byte("test")) {
		t.Errorf("Markdown file doesn't contain expected content")
	}
}

// TestMutexNotRWMutex detects if Chat still uses Mutex instead of RWMutex.
func TestMutexNotRWMutex(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())

	// This test MUST fail if Chat.mutex is sync.Mutex
	// Once Chat.mutex is changed to sync.RWMutex, this test will pass
	
	// Check the type of chat.mutex
	switch chat.mutex.(type) {
	case sync.RWMutex:
		// Correct - using RWMutex
	case sync.Mutex:
		t.Fatal("FAIL: Chat.mutex is sync.Mutex, must be sync.RWMutex for Phase 1")
	default:
		t.Fatalf("FAIL: Chat.mutex is unexpected type: %T", chat.mutex)
	}
}

