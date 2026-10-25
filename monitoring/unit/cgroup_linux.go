//go:build linux

package monitoring

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// cgroup helpers (cgroup patch, based on official 1.5.11).
//
// All helpers fail soft: any read error or "no limit" returns ok=false and
// callers fall back to the upstream /proc-based behaviour.
//
// Paths are resolved from /proc/self/cgroup so the helpers work both with a
// private cgroup namespace (where /sys/fs/cgroup is the process's own group)
// and on hosts where the full hierarchy path is visible.

func cgroupV2Dir() string {
	if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.SplitN(line, ":", 3)
			if len(parts) == 3 && parts[0] == "0" {
				p := filepath.Join("/sys/fs/cgroup", parts[2])
				if st, err := os.Stat(p); err == nil && st.IsDir() {
					return p
				}
			}
		}
	}
	return "/sys/fs/cgroup"
}

func cgroupV1Dir(subsys string) string {
	if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			parts := strings.SplitN(line, ":", 3)
			if len(parts) != 3 {
				continue
			}
			for _, s := range strings.Split(parts[1], ",") {
				if s == subsys {
					p := filepath.Join("/sys/fs/cgroup", subsys, parts[2])
					if st, err := os.Stat(p); err == nil && st.IsDir() {
						return p
					}
				}
			}
		}
	}
	return filepath.Join("/sys/fs/cgroup", subsys)
}

func readTrimmedFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func readUintFile(path string) (uint64, error) {
	s, err := readTrimmedFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(s, 10, 64)
}

func readIntFile(path string) (int64, error) {
	s, err := readTrimmedFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(s, 10, 64)
}

// readStatKey reads one "key value" line from a cgroup stat file.
func readStatKey(path, key string) (uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == key {
			return strconv.ParseUint(f[1], 10, 64)
		}
	}
	return 0, os.ErrNotExist
}

// cgroupMemoryLimit returns the effective memory limit in bytes.
// v2 memory.max holding "max" (or a v1 limit >= 2^62) means unlimited.
func cgroupMemoryLimit() (uint64, bool) {
	if s, err := readTrimmedFile(filepath.Join(cgroupV2Dir(), "memory.max")); err == nil {
		if s == "max" {
			return 0, false
		}
		if v, err := strconv.ParseUint(s, 10, 64); err == nil && v > 0 {
			return v, true
		}
		return 0, false
	}
	if v, err := readUintFile(filepath.Join(cgroupV1Dir("memory"), "memory.limit_in_bytes")); err == nil && v > 0 && v < (1<<62) {
		return v, true
	}
	return 0, false
}

// cgroupMemoryUsed returns current memory usage minus inactive file cache
// (docker-stats style accounting), clamped at 0.
func cgroupMemoryUsed() (uint64, bool) {
	dir := cgroupV2Dir()
	if cur, err := readUintFile(filepath.Join(dir, "memory.current")); err == nil {
		var inactive uint64
		if v, err := readStatKey(filepath.Join(dir, "memory.stat"), "inactive_file"); err == nil {
			inactive = v
		}
		if cur > inactive {
			return cur - inactive, true
		}
		return 0, true
	}
	dir1 := cgroupV1Dir("memory")
	if cur, err := readUintFile(filepath.Join(dir1, "memory.usage_in_bytes")); err == nil {
		var inactive uint64
		if v, err := readStatKey(filepath.Join(dir1, "memory.stat"), "total_inactive_file"); err == nil {
			inactive = v
		}
		if cur > inactive {
			return cur - inactive, true
		}
		return 0, true
	}
	return 0, false
}

// cgroupCPULimit returns the CPU quota in cores (e.g. 0.25), or ok=false
// when the cgroup has no CPU limit.
func cgroupCPULimit() (float64, bool) {
	if s, err := readTrimmedFile(filepath.Join(cgroupV2Dir(), "cpu.max")); err == nil {
		f := strings.Fields(s)
		if len(f) == 2 {
			if f[0] == "max" {
				return 0, false
			}
			q, e1 := strconv.ParseFloat(f[0], 64)
			p, e2 := strconv.ParseFloat(f[1], 64)
			if e1 == nil && e2 == nil && q > 0 && p > 0 {
				return q / p, true
			}
		}
		return 0, false
	}
	q, e1 := readIntFile(filepath.Join(cgroupV1Dir("cpu"), "cpu.cfs_quota_us"))
	p, e2 := readIntFile(filepath.Join(cgroupV1Dir("cpu"), "cpu.cfs_period_us"))
	if e1 == nil && e2 == nil && q > 0 && p > 0 {
		return float64(q) / float64(p), true
	}
	return 0, false
}

// cgroupCPUUsageUSec returns cumulative CPU time of the cgroup in
// microseconds (v2 cpu.stat usage_usec; v1 cpuacct.usage is nanoseconds).
func cgroupCPUUsageUSec() (uint64, bool) {
	if v, err := readStatKey(filepath.Join(cgroupV2Dir(), "cpu.stat"), "usage_usec"); err == nil {
		return v, true
	}
	if v, err := readUintFile(filepath.Join(cgroupV1Dir("cpuacct"), "cpuacct.usage")); err == nil {
		return v / 1000, true
	}
	return 0, false
}
