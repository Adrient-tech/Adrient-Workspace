package cloudimport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/oauth2"
)

// apiClient is an authenticated JSON client for one vendor API.
type apiClient struct {
	http  *http.Client // already carries the OAuth token (oauth2.Transport)
	base  string       // e.g. https://www.googleapis.com
	retry RetryPolicy
}

func newAPIClient(ctx context.Context, ts oauth2.TokenSource, base string, hc *http.Client) *apiClient {
	if hc != nil {
		ctx = context.WithValue(ctx, oauth2.HTTPClient, hc)
	}
	return &apiClient{http: oauth2.NewClient(ctx, ts), base: strings.TrimRight(base, "/")}
}

func (a *apiClient) url(p string, q url.Values) string {
	u := p
	if !strings.HasPrefix(p, "http://") && !strings.HasPrefix(p, "https://") {
		u = a.base + p
	}
	if len(q) > 0 {
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u += sep + q.Encode()
	}
	return u
}

func (a *apiClient) get(ctx context.Context, u string) (*http.Response, error) {
	return a.retry.DoRetry(ctx, a.http, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodGet, u, nil)
		if err == nil {
			req.Header.Set("Accept", "application/json")
		}
		return req, err
	})
}

func (a *apiClient) getJSON(ctx context.Context, p string, q url.Values, out any) error {
	u := a.url(p, q)
	resp, err := a.get(ctx, u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", p, err)
	}
	return nil
}

func (a *apiClient) open(ctx context.Context, p string, q url.Values) (io.ReadCloser, error) {
	resp, err := a.get(ctx, a.url(p, q))
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// cloneQ copies q and sets k=v.
func cloneQ(q url.Values, k, v string) url.Values {
	out := url.Values{}
	for kk, vv := range q {
		out[kk] = append([]string(nil), vv...)
	}
	if v != "" {
		out.Set(k, v)
	}
	return out
}
