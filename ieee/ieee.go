// Package ieee is the library behind the ieee command line:
// the HTTP client, request shaping, and the typed data models for IEEE Xplore
// paper metadata.
//
// Data source: CrossRef public API (api.crossref.org) filtered to IEEE papers
// (DOI prefix 10.1109). CrossRef is the policy-compliant, open alternative to
// ieeexplore.ieee.org which blocks datacenter IPs via CloudFront WAF.
//
// No API key is required. The CrossRef polite pool is used when --mailto is set.
package ieee

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// CrossRefHost is the CrossRef API hostname.
const CrossRefHost = "api.crossref.org"

// CrossRefBaseURL is the CrossRef API root.
const CrossRefBaseURL = "https://" + CrossRefHost

// IEEEHost is the IEEE Xplore hostname (used for URL construction only).
const IEEEHost = "ieeexplore.ieee.org"

// IEEEDoiPrefix is the DOI prefix for all IEEE publications.
const IEEEDoiPrefix = "10.1109"

// DefaultUserAgent identifies this client per CrossRef etiquette.
const DefaultUserAgent = "ieee/dev (+https://github.com/tamnd/ieee-cli)"

// Config holds constructor parameters for Client.
type Config struct {
	// CrossRefBaseURL is the CrossRef API root. Override in tests.
	CrossRefBaseURL string
	UserAgent       string
	// Mailto is the contact email for the CrossRef polite pool.
	// Optional but recommended; set via --mailto or IEEE_MAILTO env var.
	Mailto  string
	Rate    time.Duration
	Retries int
	Timeout time.Duration
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		CrossRefBaseURL: CrossRefBaseURL,
		UserAgent:       DefaultUserAgent,
		Rate:            150 * time.Millisecond, // ~6 rps, well under 50 rps polite limit
		Retries:         3,
		Timeout:         30 * time.Second,
	}
}

// Client is a rate-limited HTTP client for the CrossRef API.
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client configured with cfg.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// Get fetches a URL with pacing and retries.
func (c *Client) Get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) ([]byte, bool, error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has elapsed since the last request.
func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}
