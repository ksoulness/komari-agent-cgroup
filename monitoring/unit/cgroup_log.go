package monitoring

import (
	"log"
	"sync"
)

var cgroupLogOnce sync.Once

// logCgroupModeOnce emits exactly one diagnostic line per process lifetime,
// on the first monitoring call, so the effective cgroup values can be
// verified from the agent log inside the container. Unavailable values
// are printed as -1.
func logCgroupModeOnce() {
	cgroupLogOnce.Do(func() {
		memTotal, memUsed := int64(-1), int64(-1)
		if v, ok := cgroupMemoryLimit(); ok {
			memTotal = int64(v)
		}
		if v, ok := cgroupMemoryUsed(); ok {
			memUsed = int64(v)
		}
		cpuQuota := -1.0
		if v, ok := cgroupCPULimit(); ok {
			cpuQuota = v
		}
		diskTotal, diskUsed := int64(-1), int64(-1)
		if info, ok := diskOverride(); ok {
			diskTotal = int64(info.Total)
			diskUsed = int64(info.Used)
		}
		log.Printf("cgroup-mode mem_total=%d mem_used=%d cpu_quota=%g disk_total=%d disk_used=%d",
			memTotal, memUsed, cpuQuota, diskTotal, diskUsed)
	})
}
