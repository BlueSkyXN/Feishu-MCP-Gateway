package gateway

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

func (b *Broker) pace(ctx context.Context, a App) error {
	rate := a.OutboundRequestsPerSecond
	if rate <= 0 {
		rate = 5
	}
	key := a.ID
	b.paceMu.Lock()
	now := time.Now()
	at := b.nextRequest[key]
	if at.Before(now) {
		at = now
	}
	if cooldown := b.cooldown[key]; cooldown.After(at) {
		at = cooldown
	}
	b.nextRequest[key] = at.Add(time.Duration(float64(time.Second) / rate))
	b.paceMu.Unlock()
	for {
		timer := time.NewTimer(time.Until(at))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		b.paceMu.Lock()
		cooldown := b.cooldown[key]
		if !cooldown.After(time.Now()) {
			b.paceMu.Unlock()
			return nil
		}
		at = cooldown
		if b.nextRequest[key].After(at) {
			at = b.nextRequest[key]
		}
		b.nextRequest[key] = at.Add(time.Duration(float64(time.Second) / rate))
		b.paceMu.Unlock()
	}

}
func (b *Broker) rememberRateLimit(a App, response *http.Response) {
	delay := time.Duration(0)
	if response.StatusCode == 429 {
		delay = time.Second
	}
	raw := response.Header.Get("Retry-After")
	if n, err := strconv.ParseFloat(raw, 64); err == nil && n >= 0 && n <= 86400 {
		delay = max(delay, time.Duration(n*float64(time.Second)))
	} else if until, err := http.ParseTime(raw); err == nil {
		delay = max(delay, min(time.Until(until), 24*time.Hour))
	}
	if delay <= 0 {
		return
	}
	b.paceMu.Lock()
	defer b.paceMu.Unlock()
	until := time.Now().Add(delay)
	if until.After(b.cooldown[a.ID]) {
		b.cooldown[a.ID] = until
	}
}
