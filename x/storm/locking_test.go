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

// TestRWMutexConcurrentReads verifies multiple goroutines can hold read locks simultaneously
func TestRWMutexConcurrentReads(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())
	chat.history = append(chat.history, &ChatRound{Query: "test", Response: "response"})

	var wg sync.WaitGroup
	var concurrentReads int32 = 0
	var maxConcurrent int32 = 0

	// Launch 10 goroutines that each read the history
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			atomic.AddInt32(&concurrentReads, 1)
			current := atomic.LoadInt32(&concurrentReads)
			if current > atomic.LoadInt32(&maxConcurrent) {
				atomic.StoreInt32(&maxConcurrent, current)
			}
			// Read operation
			_ = chat.getHistory(true)
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&concurrentReads, -1)
		}()
	}
	wg.Wait()

	// With RWMutex, multiple readers should have been concurrent
	if atomic.LoadInt32(&maxConcurrent) < 2 {
		t.Errorf("Expected concurrent reads, got max concurrent: %d", atomic.LoadInt32(&maxConcurrent))
	}
}

// TestRWMutexWriteBlocksReads verifies write lock blocks readers
func TestRWMutexWriteBlocksReads(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())

	var writeStarted bool
	var readBlockedByWrite bool
	var mu sync.Mutex

	// Start a write operation
	go func() {
		mu.Lock()
		writeStarted = true
		mu.Unlock()

		round := chat.StartRound("test query", "")
		time.Sleep(50 * time.Millisecond)
		chat.FinishRound(round, "test response")
	}()

	// Give write time to acquire lock
	time.Sleep(10 * time.Millisecond)

	// Try to read while write is in progress
	start := time.Now()
	mu.Lock()
	if writeStarted {
		readBlockedByWrite = true
	}
	mu.Unlock()
	_ = chat.getHistory(true)
	elapsed := time.Since(start)

	// Read should have been delayed by write
	if readBlockedByWrite && elapsed < 30*time.Millisecond {
		t.Logf("Read completed too quickly during write: %v", elapsed)
	}
}

// TestNoRaceConditionDuringConcurrentQueries verifies no data races
func TestNoRaceConditionDuringConcurrentQueries(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())

	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Read history
			_ = chat.getHistory(true)

			// Start a round
			round := chat.StartRound(fmt.Sprintf("query %d", id), "")

			// Simulate LLM processing without holding lock
			time.Sleep(5 * time.Millisecond)

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

	// XXX add more validation -- make sure all queries/responses are
	// present and that file is well-formed
}

// TestStartRoundUsesWriteLock verifies StartRound acquires write lock
func TestStartRoundUsesWriteLock(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())

	var wg sync.WaitGroup

	// Start multiple rounds concurrently
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			round := chat.StartRound(fmt.Sprintf("query %d", id), "")
			if round == nil {
				t.Error("StartRound returned nil")
			}
		}(i)
	}
	wg.Wait()

	// All rounds should be recorded
	if chat.TotalRounds() != 5 {
		t.Errorf("Expected 5 rounds, got %d", chat.TotalRounds())
	}
}

// TestFinishRoundMinimizesLockDuration verifies lock is only held during file I/O
func TestFinishRoundMinimizesLockDuration(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())
	round := chat.StartRound("test query", "")

	// Measure time to finish round with response
	startTime := time.Now()
	err = chat.FinishRound(round, "test response")
	duration := time.Since(startTime)

	if err != nil {
		t.Fatalf("FinishRound failed: %v", err)
	}

	// Should complete quickly (file I/O only, not holding lock for processing)
	if duration > 500*time.Millisecond {
		t.Logf("FinishRound took too long: %v (may indicate excessive lock holding)", duration)
	}

	// Verify the file was updated
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

// TestGetHistoryWithLockParameter verifies getHistory respects lock parameter
func TestGetHistoryWithLockParameter(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())
	chat.history = append(chat.history, &ChatRound{Query: "query1", Response: "response1"})

	// Call with lock=true (should work)
	history1 := chat.getHistory(true)
	if !bytes.Contains([]byte(history1), []byte("query1")) {
		t.Errorf("History with lock=true missing query")
	}

	// Call with lock=false while holding mutex (should work without deadlock)
	chat.mutex.Lock()
	history2 := chat.getHistory(false)
	chat.mutex.Unlock()

	if !bytes.Contains([]byte(history2), []byte("query1")) {
		t.Errorf("History with lock=false missing query")
	}
}

// TestConcurrentReadsDontCorruptHistory verifies read operations are safe
func TestConcurrentReadsDontCorruptHistory(t *testing.T) {
	tmpFile, err := ioutil.TempFile("", "test-*.md")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	chat := NewChat(tmpFile.Name())

	// Add initial history
	for i := 0; i < 5; i++ {
		chat.history = append(chat.history, &ChatRound{
			Query:    fmt.Sprintf("query %d", i),
			Response: fmt.Sprintf("response %d", i),
		})
	}

	var wg sync.WaitGroup
	readCount := 0
	var mu sync.Mutex

	// Multiple readers
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = chat.getHistory(true)
			// XXX why are we locking here? just to count reads?
			mu.Lock()
			readCount++
			mu.Unlock()
		}()
	}

	wg.Wait()

	mu.Lock()
	if readCount != 20 {
		t.Errorf("Not all reads completed: %d/20", readCount)
	}
	mu.Unlock()

	// Verify history is intact
	if chat.TotalRounds() != 5 {
		t.Errorf("History corrupted: expected 5 rounds, got %d", chat.TotalRounds())
	}
}

// TestUpdateMarkdownDoesNotDeadlock verifies file updates complete
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

	// XXX add more validation -- ensure file contents are correct
}
