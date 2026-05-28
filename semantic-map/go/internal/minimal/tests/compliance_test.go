package minimal_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DiyazY/di-agent/compliance"
	"github.com/DiyazY/di-agent/internal/minimal"
	"github.com/DiyazY/di-agent/pkg/contracts"
)

func TestInMemoryStorageCompliance(t *testing.T) {
	compliance.RunStorageCompliance(t, func(t *testing.T) contracts.StorageContract {
		return minimal.NewInMemoryStorage()
	})
}

func TestEMAUpdaterCompliance(t *testing.T) {
	compliance.RunUpdaterCompliance(t, func(t *testing.T) (contracts.UpdaterContract, contracts.StorageContract) {
		s := minimal.NewInMemoryStorage()
		u := minimal.NewEMAUpdater(s, 0.2, 500)
		return u, s
	})
}

func TestCgroupCollectorCompliance(t *testing.T) {
	compliance.RunCollectorCompliance(t, func(t *testing.T) contracts.CollectorContract {
		root := newFakeCgroupRoot(t)
		c := minimal.NewCgroupCollector("test-node", root)
		// Warm up: first Collect() stores the initial snapshot.
		// The second call (from the compliance suite) will have a non-zero
		// delta and return CPU samples alongside memory.
		c.Collect() //nolint:errcheck
		time.Sleep(2 * time.Millisecond)
		return c
	})
}

// newFakeCgroupRoot creates a temp directory with valid cgroups v2 files
// so CgroupCollector can be exercised without a real kernel cgroup mount.
func newFakeCgroupRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	mustWrite(t, filepath.Join(root, "cpu.stat"),
		"usage_usec 1000000\n"+
			"user_usec 800000\n"+
			"system_usec 200000\n"+
			"nr_periods 1000\n"+
			"nr_throttled 50\n"+
			"throttled_usec 25000\n",
	)
	mustWrite(t, filepath.Join(root, "memory.current"), "2147483648\n") // 2 GB
	mustWrite(t, filepath.Join(root, "memory.max"), "8589934592\n")      // 8 GB

	return root
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("mustWrite %s: %v", path, err)
	}
}
