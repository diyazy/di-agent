// Package parquet streams Netdata long-format parquet rows for the replay
// tool.
//
// The shipping dataset (multidimensional-analysis/data/raw/{kd}/{test}_runN.parquet)
// uses a fixed column schema:
//
//	hostname       string
//	chart_id       string
//	chart_family   string
//	chart_context  string
//	units          string
//	metric_id      string
//	metric_name    string
//	value          float64
//	relative_time  int64
//
// Reader opens a parquet file and yields rows as typed Go structs, one at a
// time. The underlying library (github.com/parquet-go/parquet-go) is fully
// streaming so memory footprint stays bounded even on the larger files
// (~390k rows / ~14 MB compressed each).
//
// Reader is single-goroutine: the caller drives iteration with Next() until
// io.EOF. Close() releases the file handle and the parquet decoder buffer.
package parquet

import (
	"errors"
	"fmt"
	"io"
	"os"

	pq "github.com/parquet-go/parquet-go"
)

// Row is the typed representation of one Netdata long-format parquet row.
//
// Field tags match the column names exactly so parquet-go's struct-mapped
// reader can pick them up. The struct is kept flat (no nesting) because every
// Netdata column is a primitive — string or numeric — and a flat shape keeps
// the hot loop in playback simple.
type Row struct {
	Hostname     string  `parquet:"hostname"`
	ChartID      string  `parquet:"chart_id"`
	ChartFamily  string  `parquet:"chart_family"`
	ChartContext string  `parquet:"chart_context"`
	Units        string  `parquet:"units"`
	MetricID     string  `parquet:"metric_id"`
	MetricName   string  `parquet:"metric_name"`
	Value        float64 `parquet:"value"`
	RelativeTime int64   `parquet:"relative_time"`
}

// Reader streams Rows from a parquet file. It owns the underlying os.File
// and parquet decoder; callers must Close it when done.
//
// Iteration model:
//
//	r, err := parquet.Open(path)
//	if err != nil { ... }
//	defer r.Close()
//	for {
//	    row, err := r.Next()
//	    if err == io.EOF { break }
//	    if err != nil { return err }
//	    // ... use row ...
//	}
type Reader struct {
	file   *os.File
	gr     *pq.GenericReader[Row]
	buffer []Row
	idx    int // next index into buffer
	closed bool
}

// readBatch is the per-call buffer size. 4096 rows is a small fraction of a
// 14 MB parquet but large enough to amortize per-call overhead from the
// parquet library.
const readBatch = 4096

// Open opens the parquet file at path and prepares it for streaming. The
// returned Reader is positioned before the first row.
func Open(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open parquet %s: %w", path, err)
	}
	gr := pq.NewGenericReader[Row](f)
	return &Reader{
		file: f,
		gr:   gr,
		// Empty buffer + non-zero idx forces the first Next() to refill.
		// (A nil slice would work too; an empty slice keeps the type
		// invariant uniform.)
		buffer: nil,
		idx:    0,
	}, nil
}

// Next returns the next row from the parquet file, or io.EOF when no more
// rows remain. The returned Row is a value copy — safe to retain across
// further Next() calls.
//
// Errors from the underlying parquet reader (other than io.EOF) are returned
// verbatim with the file path attached so the caller can identify which
// parquet failed.
func (r *Reader) Next() (*Row, error) {
	if r.closed {
		return nil, errors.New("parquet.Reader: Next() called after Close()")
	}
	if r.buffer == nil || r.idx >= len(r.buffer) {
		// Refill: Read may return (n>0, io.EOF) on the last batch,
		// which means "use these and then stop." We capture both halves.
		// Grow the buffer back to its full capacity before each Read so
		// we hand the library a fresh writable slice; the previous batch
		// was reduced via reslice (buffer[:n]) and so its underlying
		// backing array is reused without allocation.
		buf := make([]Row, readBatch)
		if cap(r.buffer) >= readBatch {
			buf = r.buffer[:readBatch]
		}
		n, err := r.gr.Read(buf)
		if n == 0 {
			if err == nil {
				return nil, io.EOF
			}
			if errors.Is(err, io.EOF) {
				return nil, io.EOF
			}
			return nil, fmt.Errorf("parquet %s: %w", r.file.Name(), err)
		}
		r.buffer = buf[:n]
		r.idx = 0
		if err != nil && !errors.Is(err, io.EOF) {
			// Real failure mid-batch; surface immediately.
			return nil, fmt.Errorf("parquet %s: %w", r.file.Name(), err)
		}
		// If err == io.EOF, we still hand out the n buffered rows; the
		// next Next() call will hit the n==0 branch above and return EOF.
	}
	row := r.buffer[r.idx]
	r.idx++
	return &row, nil
}

// NumRows returns the total row count the parquet file advertises in its
// metadata. Useful for progress bars; does not consume the stream.
func (r *Reader) NumRows() int64 {
	return r.gr.NumRows()
}

// Close releases the parquet decoder and underlying file. Safe to call
// multiple times — only the first call has effect.
func (r *Reader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	cerr := r.gr.Close()
	ferr := r.file.Close()
	if cerr != nil {
		return cerr
	}
	return ferr
}
