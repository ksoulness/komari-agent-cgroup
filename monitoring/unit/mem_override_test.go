package monitoring

import (
	"testing"

	pkg_flags "github.com/komari-monitor/komari-agent/cmd/flags"
)

// The --mem-total-mb override must pin Ram().Total to the exact plan
// value and clamp Used so it never exceeds the total, regardless of
// whether the underlying numbers come from cgroup or /proc.
func TestMemTotalOverride(t *testing.T) {
	cfg := pkg_flags.GlobalConfig
	old := cfg.MemTotalMB
	defer func() { cfg.MemTotalMB = old }()

	cfg.MemTotalMB = 0
	if v := memTotalOverride(); v != 0 {
		t.Fatalf("memTotalOverride() = %d with flag unset, want 0", v)
	}
	base := Ram()
	if base.Total == 0 {
		t.Fatalf("base Ram() returned total 0 on this host")
	}

	cfg.MemTotalMB = 256
	if v := memTotalOverride(); v != 256*1024*1024 {
		t.Fatalf("memTotalOverride() = %d, want %d", v, 256*1024*1024)
	}
	info := Ram()
	if info.Total != 256*1024*1024 {
		t.Fatalf("Ram().Total = %d, want exactly %d", info.Total, 256*1024*1024)
	}
	if info.Used > info.Total {
		t.Fatalf("Ram().Used = %d exceeds total %d", info.Used, info.Total)
	}
	if info.Used != base.Used && base.Used <= info.Total {
		t.Fatalf("Ram().Used = %d, want unchanged base used %d", info.Used, base.Used)
	}

	// Clamp path: with a 1 MiB total, any realistic base usage exceeds
	// the total, so Used must be clamped down to exactly the total.
	if base.Used > 1*1024*1024 {
		cfg.MemTotalMB = 1
		clamped := Ram()
		if clamped.Total != 1*1024*1024 {
			t.Fatalf("clamped total = %d, want %d", clamped.Total, 1*1024*1024)
		}
		if clamped.Used != clamped.Total {
			t.Fatalf("clamped used = %d, want == total %d", clamped.Used, clamped.Total)
		}
	}
}
