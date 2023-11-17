package grokker

import (
	"io"
	"math"
	"os"

	. "github.com/stevegt/goadapt"
)

// copyFile copies a file from src to dst
func copyFile(src, dst string) (err error) {
	defer Return(&err)
	// open src file
	srcfh, err := os.Open(src)
	Ck(err)
	defer srcfh.Close()
	// ensure dst file does not exist
	_, err = os.Stat(dst)
	if err == nil {
		Fpf(os.Stderr, "Error: %s already exists\n", dst)
		os.Exit(1)
	}
	// open dst file with same mode as src
	fi, err := srcfh.Stat()
	Ck(err)
	dstfh, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode())
	Ck(err)
	defer dstfh.Close()
	// copy
	_, err = io.Copy(dstfh, srcfh)
	Ck(err)
	return
}

// similarity returns the cosine similarity between two embeddings.
func similarity(a, b []float64) float64 {
	var dot, magA, magB float64
	for i := range a {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}
