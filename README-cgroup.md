# komari-agent cgroup patch (based on official 1.5.11)

A patched build of [komari-monitor/komari-agent](https://github.com/komari-monitor/komari-agent)
tag `1.5.11` for use inside resource-limited containers (e.g. Botkeep /
Pterodactyl), where the stock agent reports the **host's** `/proc` and
statfs numbers instead of the container's real quotas.

Version string of this build: `1.5.11-cgroup.2` (`.1` was the first cgroup
build; `.2` adds the `--mem-total-mb` plan-value override below).

## What changes

All changes fail soft: any cgroup read error or "no limit" silently falls
back to the upstream behaviour. No protocol changes, no changes to
existing flags.

| Metric | Stock behaviour | Patched behaviour |
| --- | --- | --- |
| Memory total/used | `/proc/meminfo` (host) | cgroup limit (`memory.max` / `memory.limit_in_bytes`) and usage (`memory.current` / `memory.usage_in_bytes` minus inactive file cache); the total can additionally be pinned to the plan value with `--mem-total-mb` (see below) |
| Swap | `/proc/meminfo` (host) | reported as 0 while a cgroup memory limit is in effect |
| CPU % | gopsutil (host, all cores) | delta of cgroup CPU counter (`cpu.stat usage_usec` / `cpuacct.usage`) normalised by wall time and quota cores from `cpu.max` / `cfs_quota_us` |
| CPU cores | host core count | quota cores — the protocol field is an **int**, so a fractional quota is reported as `max(1, round(quota))` (e.g. 0.25 core shows as 1 core; the usage % stays quota-accurate) |
| Disk total/used | statfs of host mounts | only when the new flags below are used |

New flags (all default to stock behaviour):

- `--disk-total-mb <int>`: report this value (MB) as disk total.
  cgroup cannot express a disk capacity quota, so the plan quota is
  supplied explicitly.
- `--disk-path <dir>`: report the recursive file size of this directory
  as disk used (default: the process working directory). Refreshed at
  most once per 60 s. Free space shown by the panel is Total − Used.
- `--mem-total-mb <int>`: pin the reported memory **total** to this
  value (MB), overriding both the cgroup limit and `/proc`. The kernel
  cgroup limit the platform enforces can be higher than the nominal
  plan quota (observed on Botkeep: a 294.4 MiB `memory.max` for a
  256 MiB plan), so use this flag to report the plan value instead.
  Used still comes from the cgroup counter (or the stock paths without
  a cgroup) and is clamped to the total, so the panel percentage never
  exceeds 100%.

  Per-node plan values in use:

  | Node | `--mem-total-mb` |
  | --- | --- |
  | eu-probe | 256 |
  | eu-ntfy | 448 |
  | eu-status | 384 |
  | eu-hook | 448 |
  | eu-watch | 512 |

cgroup v2 is tried first, then v1 paths; paths are resolved via
`/proc/self/cgroup` so private cgroup namespaces work.

On the first monitoring call the agent logs one diagnostic line, e.g.:

```
cgroup-mode mem_total=268435456 mem_used=73400320 cpu_quota=0.25 disk_total=536870912 disk_used=123456789
```

Unavailable values are printed as `-1`.

## Files touched

- `monitoring/unit/cgroup_linux.go` (new) — cgroup v1/v2 readers
- `monitoring/unit/cgroup_other.go` (new) — non-Linux stubs
- `monitoring/unit/cgroup_log.go` (new) — one-time `cgroup-mode` log line
- `monitoring/unit/cgroup_linux_test.go` (new) — graceful-fallback and disk-override tests
- `monitoring/unit/mem_override_test.go` (new) — `--mem-total-mb` exact-total and Used-clamp tests
- `monitoring/unit/mem.go` — `Ram()` cgroup branch plus `--mem-total-mb` plan-value override (Used clamped to Total), `Swap()` zeroed in cgroup mode
- `monitoring/unit/cpu.go` — `Cpu()` quota-relative percent sampler, `CpuStaticInfo()` quota cores
- `monitoring/unit/disk.go` — `Disk()` override (`--disk-total-mb` / `--disk-path`, 60 s cache)
- `cmd/flags/flag.go`, `cmd/root.go` — the three new flags
  (`AGENT_DISK_TOTAL_MB` / `AGENT_DISK_PATH` / `AGENT_MEM_TOTAL_MB` env equivalents included)

Because the monitoring record loop and the basic-info uploader share
`unit.Ram()/Swap()/Cpu()/Disk()`, one patch point covers both.

## Build

```sh
go build -trimpath \
  -ldflags="-s -w -X github.com/komari-monitor/komari-agent/update.CurrentVersion=1.5.11-cgroup.2" \
  -o komari-agent-linux-amd64 .
# with CGO_ENABLED=0 GOOS=linux GOARCH=amd64 for the release binary
go vet ./...
go test ./monitoring/unit/ -run 'TestCgroupHelpersGraceful|TestDiskOverride|TestMemTotalOverride' -v
./komari-agent-linux-amd64 --disk-total-mb 512 check-mem   # prints the cgroup-mode line
./komari-agent-linux-amd64 --mem-total-mb 256 check-mem   # cgroup-mode mem_total=268435456
```

## Rollback

Nothing on the server side changes. To roll back a node: delete the
patched `komari-agent` binary (and `.komari-agent.pid`) from the app
directory and restart — the stock wrapper re-downloads the official
1.5.11 binary, and the node reports host numbers again. Removing the
`--disk-total-mb` flag alone disables only the disk override.
