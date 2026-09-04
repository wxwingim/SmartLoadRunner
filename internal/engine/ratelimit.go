package engine

import (
	"context"
	"math"
	"sync"
	"time"
)

// rateLimiter — простой токенный бакет: ёмкость rate токенов,
// пополнение rate токенов/сек (пересчитывается каждую итерацию ожидания).
type rateLimiter struct {
	mu     sync.Mutex
	tokens float64
	rate   float64 // токенов в секунду
	last   time.Time
}

func newRateLimiter(rate int) *rateLimiter {
	return &rateLimiter{tokens: float64(rate), rate: float64(rate), last: time.Now()}
}

// wait блокируется до получения токена; при отмене ctx — возвращает ctx.Err().
func (rl *rateLimiter) wait(ctx context.Context) error {
	for {
		rl.mu.Lock()
		now := time.Now()
		rl.tokens = math.Min(rl.rate, rl.tokens+now.Sub(rl.last).Seconds()*rl.rate)
		rl.last = now
		if rl.tokens >= 1 {
			rl.tokens--
			rl.mu.Unlock()
			return nil
		}
		// сколько ждать до следующего токена
		need := (1 - rl.tokens) / rl.rate
		rl.mu.Unlock()

		t := time.NewTimer(time.Duration(need * float64(time.Second)))
		select {
		case <-ctx.Done():
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
			return ctx.Err() //nolint:wrapcheck // ctx.Err() — уже наша ошибка
		case <-t.C:
		}
	}
}
