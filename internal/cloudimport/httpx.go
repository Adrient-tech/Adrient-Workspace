package cloudimport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// RetryPolicy controls how vendor API calls back off on throttling.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	// Sleep is replaceable in tests.
	Sleep func(ctx context.Context, d time.Duration) error
}

// DefaultRetry honours Retry-After and otherwise backs off exponentially with jitter.
var DefaultRetry = RetryPolicy{MaxAttempts: 6, BaseDelay: 500 * time.Millisecond, MaxDelay: 60 * time.Second}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// HTTPError is a non-2xx vendor response.
type HTTPError struct {
	Status int
	Body   string
	URL    string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s: HTTP %d %s", e.URL, e.Status, e.Body)
}

// retryAfter parses Retry-After (seconds or HTTP date).
func retryAfter(h string) (time.Duration, bool) {
	if h == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(h); err == nil && n >= 0 {
		return time.Duration(n) * time.Second, true
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := time.Until(t); d > 0 {
			return d, true
		}
		return 0, true
	}
	return 0, false
}

// DoRetry sends the request built by mk (called once per attempt so bodies can
// be replayed), retrying 429/502/503/504 with backoff. The caller closes the
// returned body. Other 4xx become *HTTPError immediately.
func (p RetryPolicy) DoRetry(ctx context.Context, c *http.Client, mk func() (*http.Request, error)) (*http.Response, error) {
	if p.MaxAttempts == 0 {
		p = DefaultRetry
	}
	sleep := p.Sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	var lastErr error
	for attempt := 0; attempt < p.MaxAttempts; attempt++ {
		req, err := mk()
		if err != nil {
			return nil, err
		}
		resp, err := c.Do(req.WithContext(ctx))
		if err == nil && resp.StatusCode < 400 {
			return resp, nil
		}
		var wait time.Duration
		retry := false
		if err != nil {
			lastErr = err
			retry = ctx.Err() == nil
		} else {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			resp.Body.Close()
			lastErr = &HTTPError{Status: resp.StatusCode, Body: string(bytes.TrimSpace(b)), URL: req.URL.Redacted()}
			switch resp.StatusCode {
			case 429, 502, 503, 504:
				retry = true
				if d, ok := retryAfter(resp.Header.Get("Retry-After")); ok {
					wait = d
				}
			}
		}
		if !retry || attempt == p.MaxAttempts-1 {
			return nil, lastErr
		}
		if wait == 0 {
			wait = p.BaseDelay << attempt
			wait += time.Duration(rand.Int63n(int64(p.BaseDelay)))
		}
		if wait > p.MaxDelay {
			wait = p.MaxDelay
		}
		if err := sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}
