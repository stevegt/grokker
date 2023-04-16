package universe

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"

	. "github.com/stevegt/goadapt"
	"github.com/stevegt/grokker/universe/kv"
)

// Universe represents the universe of documents, chunks, and
// embeddings. It uses a local kv database to cache the results of
// calls to external services such as the OpenAI API. For the OpenAI
// embeddings API; once a chunk of context has been sent to the API,
// it and its embedding are cached in the db so we don't have to call
// the API for that chunk again.  The db also stores a merkle tree for
// each document path, so we can efficiently detect when a document
// has changed and update the embeddings for the changed chunks.

type Universe struct {
	kv *kv.Db
}

// Open opens the universe, creating kv db and its buckets if it doesn't exist.
//
// Buckets:
// - name: chunk, key: content hash, value: content chunk
// - name: embedding, key: content hash, value: embedding
// - name: root, key: document path, value: merkle hash
// - name: tree, key: merkle hash, value: list of hashes of trees or chunks
func Open(path string) (u *Universe, err error) {
	defer Return(&err)
	u = &Universe{}
	u.kv, err = kv.Open(path)
	Ck(err)
	// start a write transaction
	tx, err := u.kv.Begin(true)
	Ck(err)
	// create buckets
	_, err = tx.MakeBucket("chunk")
	Ck(err)
	_, err = tx.MakeBucket("embedding")
	Ck(err)
	_, err = tx.MakeBucket("root")
	Ck(err)
	_, err = tx.MakeBucket("tree")
	Ck(err)
	err = tx.Commit()
	Ck(err)
	return
}

// Close closes the universe.
func (u *Universe) Close() error {
	return u.kv.Close()
}

// compress compresses data using zlib.
func compress(data []byte) (compressed []byte, err error) {
	defer Return(&err)
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err = w.Write(data)
	Ck(err)
	err = w.Close()
	Ck(err)
	return buf.Bytes(), nil
}

// decompress decompresses data using zlib.
func decompress(data []byte) (decompressed []byte, err error) {
	defer Return(&err)
	r, err := zlib.NewReader(bytes.NewReader(data))
	Ck(err)
	defer r.Close()
	return ioutil.ReadAll(r)
}

// putChunk adds or replaces a chunk, returning the hash.
func (u *Universe) putChunk(buf []byte) (hash string, err error) {
	defer Return(&err)
	tx, err := u.kv.Begin(true)
	Ck(err)
	chunk := NewChunk(buf)
	hash, compressed, err := chunk.MarshalCompressed()
	Ck(err)
	err = tx.Put("chunk", hash, compressed)
	Ck(err)
	err = tx.Commit()
	Ck(err)
	return
}

// Chunk is a chunk of content.
type Chunk struct {
	Content   []byte
	Embedding []float64
	hash      string
}

// NewChunk creates a new chunk from content.
func NewChunk(content []byte) *Chunk {
	return &Chunk{
		Content: content,
	}
}

// Hash returns the hash of a chunk.
func (c *Chunk) Hash() string {
	if c.hash == "" {
		hash := sha256.Sum256(c.contentWithHeader())
		c.hash = hex.EncodeToString(hash[:])
	}
	return c.hash
}

// contentWithHeader returns the content with a header.
func (c *Chunk) contentWithHeader() (data []byte) {
	header := fmt.Sprintf("chunk %d\x00", len(c.Content))
	data = append([]byte(header), c.Content...)
	return
}

// MarshalCompressed marshals a chunk to compressed data for storing
// in the database. Like git, we add an object header, then compute
// the hash of the header and content, then compress the header and
// content.
func (c *Chunk) MarshalCompressed() (hash string, data []byte, err error) {
	defer Return(&err)
	uncompressed := c.contentWithHeader()
	data, err = compress(uncompressed)
	Ck(err)
	return
}

// UnmarshalCompressed unmarshals a chunk from compressed data.
func UnmarshalCompressed(data []byte) (c *Chunk, err error) {
	defer Return(&err)
	data, err = decompress(data)
	Ck(err)
	// parse header
	i := bytes.IndexByte(data, 0)
	if i < 0 {
		return nil, fmt.Errorf("invalid chunk header")
	}
	header := string(data[:i])
	data = data[i+1:]
	if !strings.HasPrefix(header, "chunk ") {
		return nil, fmt.Errorf("invalid chunk header")
	}
	n, err := strconv.Atoi(header[6:])
	if err != nil {
		return nil, fmt.Errorf("invalid chunk header")
	}
	if len(data) != n {
		return nil, fmt.Errorf("invalid chunk header")
	}
	// parse content
	c = &Chunk{
		Content: data,
	}
	return
}

/*
// GetContent gets the content for a hash.
func (db *Db) GetContent(hash string) (content []byte, err error) {
	defer Return(&err)
	tx, err := db.kv.Begin(false)
	Ck(err)
	defer tx.Rollback()
	content, err = tx.Get("content", hash)
	Ck(err)
	return
}

// GetEmbedding gets the embedding for a hash.
func (db *Db) GetEmbedding(hash string) (embedding []byte, err error) {
	defer Return(&err)
	tx, err := db.kv.Begin(false)
	Ck(err)
	defer tx.Rollback()
	embedding, err = tx.Get("embedding", hash)
	Ck(err)
	return
}

// PutEmbedding adds or replaces an embedding.
func (db *Db) PutEmbedding(hash string, embedding []byte) (err error) {
	defer Return(&err)
	tx, err := db.kv.Begin(true)
	Ck(err)
	err = tx.Put("embedding", hash, embedding)
	Ck(err)
	err = tx.Commit()
	Ck(err)
	return
}

// GetRoot gets the merkle root for a document path.  It returns "" if
// not found.
func (db *Db) GetRoot(path string) (root string, err error) {
	defer Return(&err)
	tx, err := db.kv.Begin(false)
	Ck(err)
	defer tx.Rollback()
	rootBin, err := tx.Get("root", path)
	Ck(err)
	if rootBin == nil {
		return "", nil
	}
	return string(rootBin), nil
}

// PutRoot adds or replaces a merkle root.
func (db *Db) PutRoot(path string, root string) (err error) {
	defer Return(&err)
	tx, err := db.kv.Begin(true)
	Ck(err)
	err = tx.Put("root", path, []byte(root))
	Ck(err)
	err = tx.Commit()
	Ck(err)
	return
}

// AddDocument adds a document to the database.
func (u *Universe) AddDocument(path string) (err error) {
	defer Return(&err)
	// create an io.Reader for the document
	fh, err := os.Open(path)
	Ck(err)
	defer fh.Close()


*/
