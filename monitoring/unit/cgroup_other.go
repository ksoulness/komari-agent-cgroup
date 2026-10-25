//go:build !linux

package monitoring

// Non-Linux stubs: cgroup mode is never active, callers use upstream logic.

func cgroupMemoryLimit() (uint64, bool)  { return 0, false }
func cgroupMemoryUsed() (uint64, bool)   { return 0, false }
func cgroupCPULimit() (float64, bool)   { return 0, false }
func cgroupCPUUsageUSec() (uint64, bool) { return 0, false }
