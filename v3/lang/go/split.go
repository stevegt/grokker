package splitter

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
)

// nodeToString converts ast.Node into a string.
// It uses the AST printer with a buffer to store the output.
func nodeToString(fset *token.FileSet, n ast.Node) string {
	var buf bytes.Buffer // Buffer to store output

	// Fprint formats node to the buffer
	err := printer.Fprint(&buf, fset, n)
	if err != nil {
		return "" // Return empty string on error
	}

	// Return the string from the buffer
	return buf.String()
}

// SplitFile splits a Go source file or text string into chunks by
// top-level declarations. It uses Go's parser and AST packages to
// parse the source file.  If txt is nil, it reads the file at path.
// Path must not be nil; it is used in the chunk headers.
func Split(path, txt string) (chunks []string, err error) {
	fset := token.NewFileSet() // Initialize a new file set

	// Parse the file or text string.
	f, err := parser.ParseFile(fset, path, txt, parser.ParseComments)
	if err != nil {
		return nil, err // Return error if parsing fails
	}

	var chunks []string // Slice to hold chunks

	// Loop over the declarations in the file
	// 1. Convert each declaration to a string
	// 2. Append the string to the chunks slice
	for _, decl := range f.Decls {
		chunks = append(chunks, nodeToString(fset, decl))
	}

	// Return chunks and nil as error
	return chunks, nil
}

// FindChunk finds a chunk in a Go source file by global name.
// It uses Go's parser to parse top-level declarations, and search
// for the desired identifier.
func (fs *FileSplitter) FindChunk(globalName string) (string, error) {
	fset := token.NewFileSet() // Initialize a new file set

	// Parse the file.
	f, err := parser.ParseFile(fset, fs.FilePath, nil, parser.ParseComments)
	if err != nil {
		// Return error if parsing fails
		return "", err
	}

	// Loop over the declarations in the file
	for _, decl := range f.Decls {
		switch dt := decl.(type) {
		// For General Declarations
		case *ast.GenDecl:
			for _, spec := range dt.Specs {
				switch st := spec.(type) {
				case *ast.TypeSpec: // if it is a type declaration
					if st.Name.Name == globalName {
						return nodeToString(fset, decl), nil
					}
				case *ast.ValueSpec: // if it is a var or const declaration
					for _, id := range st.Names {
						if id.Name == globalName {
							return nodeToString(fset, decl), nil
						}
					}
				}
			}
			// For Function Declarations
		case *ast.FuncDecl:
			if dt.Name.Name == globalName {
				return nodeToString(fset, decl), nil
			}
		}
	}
	// Return empty string and nil error if no matching globalName is found
	return "", nil
}
