package vecdb

import (
	_ "embed" // required for embedding sqlite_vec
	"encoding/json"
	"errors"
	"fmt"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/ncruces" //nolint:revive //common alias
	"github.com/ncruces/go-sqlite3"
)

type VectorDB struct {
	db   *sqlite3.Conn
	dim  int
	path string
}

type Opt func(*VectorDB)

func WithPath(p string) Opt {
	return func(v *VectorDB) {
		v.path = p
	}
}

var (
	ErrInvalidDim  = errors.New("invalid dim: must be > 0")
	ErrDimMismatch = errors.New("vector dim mismatch")
)

const schema = `
CREATE TABLE IF NOT EXISTS
	chunks (
		rowid INTEGER PRIMARY KEY,
		content TEXT NOT NULL,
		meta TEXT CHECK (json_valid(meta))
	);

CREATE VIRTUAL TABLE IF NOT EXISTS vec_items USING vec0(embedding float[%d]);
`

func New(dim int, opts ...Opt) (*VectorDB, error) {
	v := &VectorDB{
		path: ":memory:",
		dim:  dim,
	}

	for _, o := range opts {
		o(v)
	}

	if v.dim <= 0 {
		return nil, ErrInvalidDim
	}

	db, err := sqlite3.Open(v.path)
	if err != nil {
		return nil, fmt.Errorf("sqlite3 open: %w", err)
	}

	err = db.Exec(
		"PRAGMA journal_mode=WAL;" +
			fmt.Sprintf(schema, v.dim))
	if err != nil {
		_ = v.db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	v.db = db

	return v, nil
}

func (v *VectorDB) Close() error {
	if v.db == nil {
		return nil
	}

	return v.db.Close()
}

type (
	Vector []float32
	rid    int64
)

type Chunk struct {
	Content string
	Vec     Vector
	Meta    any
}

type SearchResult struct {
	ID       rid
	Distance float64
	Content  string
	Meta     json.RawMessage
}

func (v *VectorDB) Insert(chunks []Chunk) (retErr error) {
	if err := v.db.Exec("BEGIN"); err != nil {
		return fmt.Errorf("begin: %w", err)
	}

	defer func() {
		if retErr != nil {
			if err := v.db.Exec("ROLLBACK"); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("rollback: %w", err))
			}
		}
	}()

	stmt, _, err := v.db.Prepare(`INSERT INTO chunks (content, meta) VALUES (?, ?) RETURNING rowid`)
	if err != nil {
		return err
	}

	defer func() {
		if err := stmt.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("close insert-chunks stmt: %w", err))
		}
	}()

	items := make(map[rid]Vector, len(chunks))

	for _, c := range chunks {
		stmt.BindText(1, c.Content)
		stmt.BindJSON(2, c.Meta)

		for stmt.Step() {
			rowid := rid(stmt.ColumnInt64(0))
			items[rowid] = c.Vec
		}

		if err := stmt.Err(); err != nil {
			return fmt.Errorf("insert chunk: %w", err)
		}

		stmt.Reset()
	}

	if err := v.insertItems(items); err != nil {
		return fmt.Errorf("insert vectors: %w", err)
	}

	if err := v.db.Exec("COMMIT"); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return nil
}

func (v *VectorDB) insertItems(items map[rid]Vector) (retErr error) {
	stmt, _, err := v.db.Prepare("INSERT INTO vec_items(rowid, embedding) VALUES (?, ?)")
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}

	defer func() {
		if err := stmt.Close(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("insert items close stmt: %w", err))
		}
	}()

	for rid, values := range items {
		if len(values) != v.dim {
			return fmt.Errorf("%w: want %d, got %d (rowid=%d)", ErrDimMismatch, v.dim, len(values), rid)
		}

		blob, err := sqlite_vec.SerializeFloat32(values)
		if err != nil {
			return err
		}

		stmt.BindInt64(1, int64(rid))
		stmt.BindBlob(2, blob)

		if err := stmt.Exec(); err != nil {
			return fmt.Errorf("exec insert rowid=%d: %w", rid, err)
		}

		stmt.Reset()
	}

	return nil
}

const searchKNNQuery = `
SELECT
	c.rowid,
	c.content,
	c.meta,
	distance
FROM
	vec_items AS v
	JOIN chunks AS c USING (rowid)
WHERE
	v.embedding MATCH ?
	AND k = ?
ORDER BY
	distance`

func (v *VectorDB) SearchKNN(q Vector, k int) ([]SearchResult, error) {
	if len(q) != v.dim {
		return nil, fmt.Errorf("%w: want %d, got %d", ErrDimMismatch, v.dim, len(q))
	}

	if k <= 0 {
		k = 5
	}

	query, err := sqlite_vec.SerializeFloat32(q)
	if err != nil {
		return nil, fmt.Errorf("serialize knn search query: %w", err)
	}

	stmt, _, err := v.db.Prepare(searchKNNQuery)
	if err != nil {
		return nil, fmt.Errorf("prepare search: %w", err)
	}
	defer stmt.Close()

	stmt.BindBlob(1, query)
	stmt.BindInt(2, k)

	out := make([]SearchResult, 0, k)

	for stmt.Step() {
		out = append(out, SearchResult{
			ID:       rid(stmt.ColumnInt64(0)),
			Content:  stmt.ColumnText(1),
			Meta:     json.RawMessage(stmt.ColumnText(2)),
			Distance: stmt.ColumnFloat(3),
		})
	}

	if err := stmt.Err(); err != nil {
		return nil, fmt.Errorf("query step: %w", err)
	}

	return out, nil
}
