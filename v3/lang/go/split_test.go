package splitter

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestNewFileSplitter(t *testing.T) {
	fs := NewFileSplitter("splitter.go")
	if fs == nil {
		t.Errorf("NewFileSplitter was incorrect, got: nil")
	}
}

func TestNodeToString(t *testing.T) {
	fset := token.NewFileSet()
	expr, err := parser.ParseExpr(`fmt.Println("hello, world!")`)
	if err != nil {
		t.Fatal(err)
	}
	want := `fmt.Println("hello, world!")`
	got := nodeToString(fset, expr)
	if strings.TrimSpace(got) != strings.TrimSpace(want) {
		t.Errorf("nodeToString was incorrect, got: %s, want: %s", got, want)
	}
}

func TestSplitFile(t *testing.T) {
	t.Skip("skipping TestSplitFile")
	fs := NewFileSplitter("splitter.go")
	chunks, err := fs.SplitFile()
	if err != nil {
		t.Errorf("SplitFile was incorrect, got: %v", err)
	}
	if len(chunks) == 0 {
		t.Errorf("SplitFile was incorrect, got: 0 chunks")
	}
}

// Adding TestFindChunk:
func TestFindChunk(t *testing.T) {
	t.Skip("skipping TestFindChunk")
	fs := NewFileSplitter("splitter.go")
	chunk, err := fs.FindChunk("FileSplitter")
	if err != nil {
		t.Errorf("FindChunk was incorrect, got: %v", err)
	}
	if chunk == "" {
		t.Errorf("FindChunk was incorrect, got: empty chunk")
	}
}
