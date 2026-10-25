package monitoring

import (
	"bufio"
	"math"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	pkg_flags "github.com/komari-monitor/komari-agent/cmd/flags"
	"github.com/shirou/gopsutil/v4/cpu"
)

var flags = pkg_flags.GlobalConfig

type CpuInfo struct {
	CPUName          string  `json:"cpu_name"`
	CPUArchitecture  string  `json:"cpu_architecture"`
	CPUCores         int     `json:"cpu_cores"`
	CPUPhysicalCores int     `json:"cpu_physical_cores"`
	CPUUsage         float64 `json:"cpu_usage"`
}

func Cpu() CpuInfo {
	logCgroupModeOnce()
	cpuinfo := CpuStaticInfo()

	// cgroup patch: when a CPU quota exists, compute usage from cgroup
	// counters (quota-relative); fall back to gopsutil on first sample,
	// counter resets or read failures.
	if quota, ok := cgroupCPULimit(); ok {
		if pct, ok := cgroupCPUPercent(quota); ok {
			cpuinfo.CPUUsage = pct
			return cpuinfo
		}
	}

	percentages, err := cpu.Percent(0, false)
	if err == nil && len(percentages) > 0 {
		cpuinfo.CPUUsage = percentages[0]
	}

	return cpuinfo
}

func CpuStaticInfo() CpuInfo {
	cpuinfo := CpuInfo{
		CPUName:          "Unknown",
		CPUArchitecture:  runtime.GOARCH,
		CPUCores:         1,
		CPUPhysicalCores: 0, // 为兼容旧版 agent，0 表示未上报或未知，避免与实际核心数混淆
		CPUUsage:         0.0,
	}

	// 优先使用 gopsutil 获取 CPU 信息，避免触发 lscpu 在部分内核上的 lockdown 日志刷屏。
	info, err := cpu.Info()
	if err == nil && len(info) > 0 {
		cpuinfo.CPUName = strings.TrimSpace(info[0].ModelName)
		if cpuinfo.CPUName == "" {
			if info[0].VendorID != "" || info[0].Family != "" {
				cpuinfo.CPUName = strings.TrimSpace(info[0].VendorID + " " + info[0].Family)
			}
		}
	}

	if cpuinfo.CPUName == "Unknown" {
		name, err := readCPUNameFromProc()
		if err == nil && name != "" {
			cpuinfo.CPUName = strings.TrimSpace(name)
		}
	}

	cores, err := cpu.Counts(true)
	if err == nil && cores > 0 {
		cpuinfo.CPUCores = cores
	}

	// cgroup patch: CPUCores is an int in the protocol, so a fractional
	// quota (e.g. 0.25 cores) cannot be represented exactly; report
	// max(1, round(quota)) when a quota is in effect.
	if quota, ok := cgroupCPULimit(); ok {
		if q := int(math.Round(quota)); q >= 1 {
			cpuinfo.CPUCores = q
		} else {
			cpuinfo.CPUCores = 1
		}
	}

	physicalCores, err := cpu.Counts(false)
	if err == nil && physicalCores > 0 {
		cpuinfo.CPUPhysicalCores = physicalCores
	}

	return cpuinfo
}

// cgroup CPU usage sampler (cgroup patch). Mirrors gopsutil's stateful
// percent algorithm: usage is derived from the delta of the cgroup CPU
// counter between two calls, normalised by wall time and quota cores.
var (
	cgCPUMu        sync.Mutex
	cgCPULastUsage uint64
	cgCPULastTime  time.Time
)

func cgroupCPUPercent(quota float64) (float64, bool) {
	usage, ok := cgroupCPUUsageUSec()
	if !ok || quota <= 0 {
		return 0, false
	}
	now := time.Now()

	cgCPUMu.Lock()
	defer cgCPUMu.Unlock()
	prevUsage, prevTime := cgCPULastUsage, cgCPULastTime
	cgCPULastUsage, cgCPULastTime = usage, now

	if prevTime.IsZero() || usage < prevUsage {
		return 0, false // first sample or counter reset: caller falls back
	}
	dWall := now.Sub(prevTime).Seconds()
	if dWall <= 0 {
		return 0, false
	}
	pct := (float64(usage-prevUsage) / 1e6) / (dWall * quota) * 100
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return pct, true
}

// readCPUNameFromProc 从 /proc/cpuinfo 读取 CPU 名称
func readCPUNameFromProc() (string, error) {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Model\t") || strings.HasPrefix(line, "Hardware\t") || strings.HasPrefix(line, "Processor\t") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}

	return "", scanner.Err()
}
