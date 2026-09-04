package engine

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/wxwingim/SmartLoadRunner/internal/models"
)

// latencyBuckets — границы бакетов по лог-шкале (мс): 1ms .. ~128s.
var latencyBuckets = []float64{
	1, 2, 4, 8, 16, 32, 64, 128, 256, 512,
	1_000, 2_000, 4_000, 8_000, 16_000, 32_000, 64_000, 128_000,
}

// Aggregator потокобезопасно копит результат за отчётное окно.
type Aggregator struct {
	mu       sync.Mutex
	requests int
	errors   int
	hist     []int64 // len(hist) == len(latencyBuckets)
}

func newAggregator() *Aggregator {
	return &Aggregator{hist: make([]int64, len(latencyBuckets))}
}

// observe принимает результат одного HTTP-запроса.
func (a *Aggregator) observe(latency time.Duration, ok bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.requests++
	if !ok {
		a.errors++
		return
	}
	ms := float64(latency) / float64(time.Millisecond)
	i := sort.SearchFloat64s(latencyBuckets, ms) // верхняя граница бакета
	if i < len(a.hist) {
		a.hist[i]++
	}
}

// percentile оценивает перцентиль p% как середину бакета, где накоплена p-я доля.
func percentile(hist []int64, p float64) float64 {
	total := 0
	for _, n := range hist {
		total += int(n)
	}
	if total == 0 {
		return 0
	}
	target := int(math.Ceil(float64(total) * p / 100))
	cum := 0
	for i, n := range hist {
		cum += int(n)
		if cum >= target {
			lo := float64(0)
			if i > 0 {
				lo = latencyBuckets[i-1]
			}
			return (lo + latencyBuckets[i]) / 2
		}
	}
	return latencyBuckets[len(latencyBuckets)-1]
}

// bucket (пере)заполняет MetricBucket и сбрасывает окно.
func (a *Aggregator) snapshot(elapsed time.Duration, activeVUs int) *models.MetricBucket {
	a.mu.Lock()
	defer a.mu.Unlock()
	secs := math.Max(elapsed.Seconds(), 1)
	m := &models.MetricBucket{
		Timestamp: time.Now().UTC(),
		RPC:       float64(a.requests) / secs,
		P50:       percentile(a.hist, 50),
		P90:       percentile(a.hist, 90),
		P99:       percentile(a.hist, 99),
		Errors:    a.errors,
		ActiveVUs: activeVUs,
	}
	a.requests, a.errors = 0, 0
	clear(a.hist)
	return m
}
