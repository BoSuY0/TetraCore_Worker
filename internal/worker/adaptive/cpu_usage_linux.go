//go:build linux

package adaptive

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// CPUTimes зберігає агреговані CPU-лічильники.
type CPUTimes struct {
	Total uint64
	Idle  uint64
}

// ReadCPUTimes читає агреговані CPU-таймси з /proc/stat.
// Total — сума всіх полів; Idle включає idle+iowait.
func ReadCPUTimes() (CPUTimes, error) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return CPUTimes{}, err
	}
	line := ""
	if idx := strings.IndexByte(string(b), '\n'); idx >= 0 {
		line = string(b[:idx])
	} else {
		line = string(b)
	}
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return CPUTimes{}, fmt.Errorf("несподіваний формат /proc/stat")
	}

	var total uint64
	var idle uint64
	for i := 1; i < len(fields); i++ {
		v, err := strconv.ParseUint(fields[i], 10, 64)
		if err != nil {
			return CPUTimes{}, err
		}
		total += v
		// idle=field4, iowait=field5 (0-indexed після "cpu": 3 та 4)
		if i == 4 || i == 5 {
			idle += v
		}
	}

	return CPUTimes{Total: total, Idle: idle}, nil
}

// CPUUsage повертає CPU usage (%) між prev і cur семплами.
// ok=false, якщо дельта не позитивна (наприклад, перший семпл).
func CPUUsage(prev, cur CPUTimes) (usage float64, ok bool) {
	if cur.Total <= prev.Total {
		return 0, false
	}
	dTotal := float64(cur.Total - prev.Total)
	dIdle := float64(cur.Idle - prev.Idle)
	if dTotal <= 0 {
		return 0, false
	}
	u := 100.0 * (1.0 - (dIdle / dTotal))
	if u < 0 {
		u = 0
	}
	if u > 100 {
		u = 100
	}
	return u, true
}
