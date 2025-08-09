package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

var (
	ErrInvalidChunkSize    = errors.New("size must be > 0")
	ErrInvalidChunkOverlap = errors.New("overlap must satisfy 0 <= overlap < size")
)

// ChunkText splits text into fixed size chunks with overlap.
func ChunkText(text string, size, overlap int) ([]string, error) {
	if size <= 0 {
		return nil, ErrInvalidChunkSize
	}

	if overlap < 0 || overlap >= size {
		return nil, ErrInvalidChunkOverlap
	}

	step := size - overlap
	r := []rune(text)
	n := len(r)

	var out []string
	for i := 0; i < n; i += step {
		end := min(i+size, n)

		out = append(out, string(r[i:end]))

		if end == n {
			break
		}
	}

	return out, nil
}

// ListFiles returns all files under dir recursively.
// If predicate is nil, all files are returned.
func ListFiles(dir string, predicate func(string) bool) ([]string, error) {
	var filenames []string

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walking %q: %w", path, err)
		}

		if d.IsDir() {
			return nil
		}

		if predicate == nil || predicate(path) {
			filenames = append(filenames, path)
		}

		return nil
	})

	return filenames, err
}

// TODO: system prompt
// TODO1: user prompt template with context

// TODO3: embedding files
//	- collect filenames
// 	- chunk files
// 	- embed files

// FIXME: make the model select popup based
// 	- add a status indication with the current model selected instead.
