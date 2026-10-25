//go:build linux

package monitoring

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	pkg_flags "github.com/komari-monitor/komari-agent/cmd/flags"
)

// The cgroup helpers must never panic and must return sane values when
// they report ok. On hosts without limits (like CI) ok=false is expected.
func TestCgroupHelpersGraceful(t *testing.T) {
	if v, ok := cgroupMemoryLimit(); ok && v == 0 {
		t.Fatalf("memory limit reported ok with 0")
	} else {
		t.Logf("memory limit: %d ok=%v", v, ok)
	}
	if v, ok := cgroupMemoryUsed(); ok && v > (1<<63) {
		t.Fatalf("memory used implausible: %d", v)
	} else {
		t.Logf("memory used: %d ok=%v", v, ok)
	}
	if v, ok := cgroupCPULimit(); ok && (v <= 0 || v > 1024) {
		t.Fatalf("cpu limit implausible: %v", v)
	} else {
		t.Logf("cpu limit: %v ok=%v", v, ok)
	}
	if v, ok := cgroupCPUUsageUSec(); ok {
		t.Logf("cpu usage usec: %d", v)
		// Sampler: first call has no baseline, second may produce a percent.
		if _, ok := cgroupCPUPercent(1); ok {
			t.Logf("first sampler call unexpectedly produced a percent")
		}
		time.Sleep(20 * time.Millisecond)
		if pct, ok := cgroupCPUPercent(1); ok && (pct < 0 || pct > 100) {
			t.Fatalf("cpu percent out of range: %v", pct)
		} else {
			t.Logf("cpu percent: %v ok=%v", pct, ok)
		}
	}
}

// Disk override must honour --disk-total-mb / --disk-path and the cache.
func TestDiskOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.bin"), make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := pkg_flags.GlobalConfig
	oldMB, oldPath := cfg.DiskTotalMB, cfg.DiskPath
	defer func() { cfg.DiskTotalMB, cfg.DiskPath = oldMB, oldPath }()
	diskOverrideOK = false

	cfg.DiskTotalMB = 16
	cfg.DiskPath = dir
	info, ok := diskOverride()
	if !ok {
		t.Fatalf("disk override not active")
	}
	if info.Total != 16*1024*1024 {
		t.Fatalf("total = %d, want %d", info.Total, 16*1024*1024)
	}
	if info.Used != 4096+2048 {
		t.Fatalf("used = %d, want %d", info.Used, 4096+2048)
	}

	cfg.DiskTotalMB = 0
	if _, ok := diskOverride(); ok {
		t.Fatalf("disk override active with total-mb=0")
	}
}
