package parquet

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	pq "github.com/parquet-go/parquet-go"
)

// writeFixture writes a small parquet file in long-format schema, suitable
// for unit tests of the reader.
func writeFixture(t *testing.T, path string, rows []Row) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	w := pq.NewGenericWriter[Row](f)
	if _, err := w.Write(rows); err != nil {
		t.Fatalf("write rows: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
}

func TestReader_StreamsAllRowsAndEOF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.parquet")
	want := []Row{
		{Hostname: "master", ChartID: "system.cpu", ChartFamily: "cpu", ChartContext: "system.cpu",
			Units: "percentage", MetricID: "idle", MetricName: "idle", Value: 99.5, RelativeTime: 0},
		{Hostname: "node_1", ChartID: "system.cpu", ChartFamily: "cpu", ChartContext: "system.cpu",
			Units: "percentage", MetricID: "user", MetricName: "user", Value: 0.21, RelativeTime: 0},
		{Hostname: "master", ChartID: "system.net", ChartFamily: "net", ChartContext: "system.net",
			Units: "kilobits/s", MetricID: "InOctets", MetricName: "InOctets", Value: 8.0, RelativeTime: 5},
	}
	writeFixture(t, path, want)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()
	if got := r.NumRows(); got != int64(len(want)) {
		t.Errorf("NumRows: got %d, want %d", got, len(want))
	}

	var got []Row
	for {
		row, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		got = append(got, *row)
	}
	if len(got) != len(want) {
		t.Fatalf("row count: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d:\n  got  %+v\n  want %+v", i, got[i], want[i])
		}
	}
}

func TestReader_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.parquet")
	writeFixture(t, path, nil)

	r, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()
	_, err = r.Next()
	if err != io.EOF {
		t.Errorf("Next on empty parquet: got %v, want io.EOF", err)
	}
}

func TestReader_NextAfterClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.parquet")
	writeFixture(t, path, []Row{{Hostname: "master"}})
	r, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	// Idempotent close.
	if err := r.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if _, err := r.Next(); err == nil {
		t.Error("Next after Close should fail")
	}
}
