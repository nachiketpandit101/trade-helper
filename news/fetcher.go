// Package news fetches recent company news from Finnhub.
//
// Endpoint reference:
//
//	https://finnhub.io/docs/api/company-news
//
// GET /api/v1/company-news?symbol={SYMBOL}&from={YYYY-MM-DD}&to={YYYY-MM-DD}&token={API_KEY}
package news

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL    = "https://finnhub.io/api/v1"
	defaultTimeout    = 10 * time.Second
	defaultLookback   = 7 * 24 * time.Hour
	maxResponseBytes  = 5 * 1024 * 1024 // 5 MiB safety cap
	defaultUserAgent  = "trade-helper/0.1 (+https://github.com/nachi/trade-helper)"
	dateLayoutFinnhub = "2006-01-02"
)

// Article is the subset of the Finnhub company-news payload that the rest
// of the app cares about. Extra fields returned by the API are ignored.
type Article struct {
	Category string `json:"category"`
	Datetime int64  `json:"datetime"` // Unix seconds
	Headline string `json:"headline"`
	ID       int64  `json:"id"`
	Image    string `json:"image"`
	Related  string `json:"related"`
	Source   string `json:"source"`
	Summary  string `json:"summary"`
	URL      string `json:"url"`
}

// Fetcher pulls company news from Finnhub. Construct it with New.
type Fetcher struct {
	apiKey   string
	baseURL  string
	client   *http.Client
	lookback time.Duration
	now      func() time.Time
}

// Option mutates a Fetcher during construction.
type Option func(*Fetcher)

// WithHTTPClient overrides the default http.Client (useful in tests).
func WithHTTPClient(c *http.Client) Option {
	return func(f *Fetcher) {
		if c != nil {
			f.client = c
		}
	}
}

// WithBaseURL overrides the Finnhub base URL (useful in tests).
func WithBaseURL(u string) Option {
	return func(f *Fetcher) {
		if u != "" {
			f.baseURL = strings.TrimRight(u, "/")
		}
	}
}

// WithLookback sets how far back to request news. Defaults to 7 days.
func WithLookback(d time.Duration) Option {
	return func(f *Fetcher) {
		if d > 0 {
			f.lookback = d
		}
	}
}

// New constructs a Fetcher bound to the given API key.
func New(apiKey string, opts ...Option) *Fetcher {
	f := &Fetcher{
		apiKey:   apiKey,
		baseURL:  defaultBaseURL,
		client:   &http.Client{Timeout: defaultTimeout},
		lookback: defaultLookback,
		now:      time.Now,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// FetchCompanyNews returns recent articles for a single ticker symbol.
// It returns a clear error when the symbol is empty, the API rejects the
// request, or the response cannot be decoded.
func (f *Fetcher) FetchCompanyNews(ctx context.Context, symbol string) ([]Article, error) {
	symbol = strings.ToUpper(strings.TrimSpace(symbol))
	if symbol == "" {
		return nil, fmt.Errorf("symbol must not be empty")
	}
	if f.apiKey == "" {
		return nil, fmt.Errorf("missing Finnhub API key")
	}

	to := f.now().UTC()
	from := to.Add(-f.lookback)

	q := url.Values{}
	q.Set("symbol", symbol)
	q.Set("from", from.Format(dateLayoutFinnhub))
	q.Set("to", to.Format(dateLayoutFinnhub))
	q.Set("token", f.apiKey)

	endpoint := fmt.Sprintf("%s/company-news?%s", f.baseURL, q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", symbol, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request finnhub for %s: %w", symbol, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read finnhub response for %s: %w", symbol, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, classifyHTTPError(symbol, resp.StatusCode, body)
	}

	var articles []Article
	if err := json.Unmarshal(body, &articles); err != nil {
		// Finnhub sometimes returns an object with an "error" field on
		// soft failures instead of a JSON array; surface that message.
		var apiErr struct {
			Error string `json:"error"`
		}
		if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil && apiErr.Error != "" {
			return nil, fmt.Errorf("finnhub error for %s: %s", symbol, apiErr.Error)
		}
		return nil, fmt.Errorf("decode finnhub response for %s: %w", symbol, err)
	}

	return articles, nil
}

// classifyHTTPError turns non-2xx responses into user-friendly errors.
func classifyHTTPError(symbol string, status int, body []byte) error {
	snippet := strings.TrimSpace(string(body))
	if len(snippet) > 200 {
		snippet = snippet[:200] + "..."
	}

	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("finnhub rejected the API key (HTTP %d) while fetching %s; check FINNHUB_API_KEY", status, symbol)
	case http.StatusTooManyRequests:
		return fmt.Errorf("finnhub rate limit hit (HTTP 429) while fetching %s; slow down or upgrade your plan", symbol)
	case http.StatusNotFound:
		return fmt.Errorf("finnhub returned 404 for %s; verify the ticker symbol", symbol)
	default:
		if snippet == "" {
			return fmt.Errorf("finnhub returned HTTP %d for %s", status, symbol)
		}
		return fmt.Errorf("finnhub returned HTTP %d for %s: %s", status, symbol, snippet)
	}
}
