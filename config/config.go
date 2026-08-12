// Package config loads runtime configuration from environment variables
// (optionally hydrated from a .env file by main).
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for trade-helper.
type Config struct {
	// APIKey is the Finnhub API key used to authenticate requests.
	APIKey string

	// Tickers is the list of stock symbols to monitor (uppercased).
	Tickers []string

	// FetchInterval is how often the app polls Finnhub in watch mode.
	FetchInterval time.Duration

	// Watch, when true, polls on FetchInterval until interrupted.
	Watch bool
}

// Load reads configuration from environment variables and returns a
// validated Config. It returns descriptive errors so the CLI can print
// actionable messages to the user.
func Load() (*Config, error) {
	apiKey := strings.TrimSpace(os.Getenv("FINNHUB_API_KEY"))
	if apiKey == "" {
		return nil, errors.New("FINNHUB_API_KEY is not set; copy .env.example to .env and add your key from https://finnhub.io/")
	}

	rawTickers := strings.TrimSpace(os.Getenv("TICKERS"))
	if rawTickers == "" {
		return nil, errors.New("TICKERS is not set; provide a comma-separated list, e.g. TICKERS=AAPL,TSLA,NVDA")
	}

	tickers, err := parseTickers(rawTickers)
	if err != nil {
		return nil, err
	}

	interval, err := parseInterval(os.Getenv("FETCH_INTERVAL"))
	if err != nil {
		return nil, err
	}

	return &Config{
		APIKey:        apiKey,
		Tickers:       tickers,
		FetchInterval: interval,
		Watch:         envTruthy(os.Getenv("WATCH")),
	}, nil
}

func envTruthy(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// parseTickers splits the comma-separated TICKERS env var, trims and
// upper-cases each entry, and de-duplicates while preserving order.
func parseTickers(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))

	for _, p := range parts {
		t := strings.ToUpper(strings.TrimSpace(p))
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}

	if len(out) == 0 {
		return nil, errors.New("TICKERS did not contain any usable symbols after trimming")
	}
	return out, nil
}

// parseInterval accepts either a Go duration string ("15m", "1h") or a
// bare integer interpreted as minutes. Empty input defaults to 15 minutes.
func parseInterval(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 15 * time.Minute, nil
	}

	if d, err := time.ParseDuration(raw); err == nil {
		if d <= 0 {
			return 0, fmt.Errorf("FETCH_INTERVAL must be positive, got %q", raw)
		}
		return d, nil
	}

	mins, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("FETCH_INTERVAL %q is not a valid duration or integer minutes", raw)
	}
	if mins <= 0 {
		return 0, fmt.Errorf("FETCH_INTERVAL must be positive, got %d", mins)
	}
	return time.Duration(mins) * time.Minute, nil
}
