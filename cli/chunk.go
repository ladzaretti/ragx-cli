package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"unicode/utf8"

	"github.com/ladzaretti/ragrat/genericclioptions"
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

func discover(files []string, matchREs []*regexp.Regexp) ([]string, error) {
	var (
		seen = make([]string, 0, 32)
		errs []error
	)

	for _, filename := range files {
		root, err := filepath.Abs(filename)
		if err != nil {
			errs = append(errs, fmt.Errorf("abs %q: %w", filename, err))
			continue
		}

		fi, err := os.Stat(root)
		if err != nil {
			errs = append(errs, fmt.Errorf("stat %q: %w", root, err))
			continue
		}

		matches := func(path string) bool {
			if len(matchREs) == 0 {
				return true
			}

			path = filepath.ToSlash(path)
			for _, re := range matchREs {
				if re.MatchString(path) {
					return true
				}
			}

			return false
		}

		if !fi.IsDir() {
			if matches(root) {
				seen = append(seen, root)
			}

			continue
		}

		files, err := ListFiles(root, matches)
		if err != nil {
			errs = append(errs, fmt.Errorf("list %q: %w", root, err))
			continue
		}

		seen = append(seen, files...)
	}

	return seen, errors.Join(errs...)
}

type fileChunks struct {
	path   string
	chunks []string
}

func chunkFiles(ctx context.Context, io *genericclioptions.IOStreams, paths []string, chunkSize, overlap int) ([]*fileChunks, error) {
	chunked := make([]*fileChunks, 0, len(paths))

	for _, path := range paths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		chunks, err := chunkFile(path, chunkSize, overlap)
		if err != nil {
			io.Warnf("chunk file: %v\n", err)
			continue
		}

		chunked = append(chunked, chunks)
	}

	return chunked, nil
}

func chunkFile(path string, chunkSize, overlap int) (*fileChunks, error) {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", path, err)
	}

	if !utf8.Valid(b) {
		return nil, fmt.Errorf("skipping non-utf-8 file: %q", path)
	}

	if bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}) { // Strip BOM
		b = b[3:]
	}

	chunks, err := ChunkText(string(b), chunkSize, overlap)
	if err != nil {
		return nil, fmt.Errorf("chunk %q: %w", path, err)
	}

	if len(chunks) == 0 {
		return nil, fmt.Errorf("skipping empty file: %q", path)
	}

	return &fileChunks{
			path:   path,
			chunks: chunks,
		},
		nil
}

func totalChunks(chunkedFiles []*fileChunks) (n int) {
	for _, cf := range chunkedFiles {
		n += len(cf.chunks)
	}

	return n
}

// TODO1: make the model select popup based
// TODO2: add a status indication with the current model selected instead.
