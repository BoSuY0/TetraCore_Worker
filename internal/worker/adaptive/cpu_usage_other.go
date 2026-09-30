//go:build !linux

package adaptive

import "fmt"

// CPUTimes зберігає агреговані CPU-лічильники.
type CPUTimes struct {
	Total uint64
	Idle  uint64
}

// ReadCPUTimes повертає помилку на платформах без підтримки /proc/stat.
func ReadCPUTimes() (CPUTimes, error) {
	return CPUTimes{}, fmt.Errorf("семплінг CPU не підтримується на цій платформі")
}

// CPUUsage повертає ok=false, якщо семплінг не підтримується.
func CPUUsage(_, _ CPUTimes) (usage float64, ok bool) {
	return 0, false
}
