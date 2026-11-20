//go:build linux

package sys

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

var SIGUSR1 = syscall.SIGUSR1

// countConnections returns the number of entries in a /proc/net/{tcp,udp}[6]
// file. Returns 0 if the file is absent (e.g. /proc/net/tcp6 when IPv6 is
// disabled) and excludes the column header line.
func countConnections(path string) (int, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	n := 0
	for sc.Scan() {
		n++
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	if n > 0 {
		n-- // first line is the column header
	}
	return n, nil
}

// GetTCPCount returns the number of active TCP connections by reading
// /proc/net/tcp and /proc/net/tcp6 when available.
func GetTCPCount() (int, error) {
	root := HostProc()
	tcp4, err := countConnections(root + "/net/tcp")
	if err != nil {
		return 0, err
	}
	tcp6, err := countConnections(root + "/net/tcp6")
	if err != nil {
		return 0, err
	}
	return tcp4 + tcp6, nil
}

// GetUDPCount returns the number of active UDP connections by reading
// /proc/net/udp and /proc/net/udp6 when available.
func GetUDPCount() (int, error) {
	root := HostProc()
	udp4, err := countConnections(root + "/net/udp")
	if err != nil {
		return 0, err
	}
	udp6, err := countConnections(root + "/net/udp6")
	if err != nil {
		return 0, err
	}
	return udp4 + udp6, nil
}

// --- CPU Utilization (Linux native) ---

var (
	cpuMu      sync.Mutex
	lastBusy   uint64
	lastIdle   uint64
	lastIOWait uint64
	hasLast    bool
)

// CPUPercentRaw returns instantaneous total CPU utilization by reading
// /proc/stat. First call initializes and returns 0; subsequent calls return
// busy/total * 100. Uses HostProc so HOST_PROC overrides (containers) apply.
func CPUPercentRaw() (float64, error) {
	cpuMu.Lock()
	defer cpuMu.Unlock()

	f, err := os.Open(HostProc("stat"))
	if err != nil {
		return 0, err
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	line, err := rd.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}
	// Expect: cpu  user nice system idle iowait irq softirq steal guest guest_nice
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, fmt.Errorf("unexpected /proc/stat format")
	}

	// Guest time is already accounted for in user/nice.
	var counters [8]uint64
	for i, field := range fields[1:min(len(fields), 9)] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid /proc/stat CPU counter: %w", err)
		}
		counters[i] = value
	}

	idle, iowait := counters[3], counters[4]
	busy := counters[0] + counters[1] + counters[2] + counters[5] + counters[6] + counters[7]
	previousBusy, previousIdle, previousIOWait := lastBusy, lastIdle, lastIOWait
	lastBusy, lastIdle, lastIOWait = busy, idle, iowait
	if !hasLast || busy < previousBusy || idle < previousIdle {
		hasLast = true
		return 0, nil
	}

	busyDelta := busy - previousBusy
	// Linux can revise iowait downwards independently of the idle counter.
	idleDelta := idle - previousIdle + iowait - min(iowait, previousIOWait)
	if busyDelta == 0 {
		return 0, nil
	}
	return float64(busyDelta) / (float64(busyDelta) + float64(idleDelta)) * 100, nil
}
