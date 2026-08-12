# trade-helper

A tiny Go CLI that pulls recent company news from
[Finnhub](https://finnhub.io/docs/api/company-news), runs a naive
keyword sentiment scan, and prints a per-ticker trade signal:

- `FAVORABLE` (green) — bullish keywords dominate
- `UNFAVORABLE` (red) — bearish keywords dominate
- `NEUTRAL` (yellow) — tie, or no signal-bearing keywords found

The app runs once and exits by default. Pass `--watch` (or set
`WATCH=true`) to poll on `FETCH_INTERVAL` until interrupted.

## Project layout

```
trade-helper/
├── main.go                  # entry point, wiring
├── config/config.go         # env loading + validation
├── news/fetcher.go          # Finnhub company-news client (stdlib only)
├── analysis/conditions.go   # keyword sentiment → signal
├── output/printer.go        # colored terminal / JSON output
├── go.mod / go.sum          # only dep: github.com/joho/godotenv
├── .env.example
└── README.md
```

## Requirements

- Go 1.22 or newer
- A free Finnhub API key — sign up at <https://finnhub.io/>

## Setup

1. Clone or copy the project, then from its root:

   ```bash
   cp .env.example .env
   ```

2. Edit `.env` and set your key + tickers:

   ```dotenv
   FINNHUB_API_KEY=ck1abc...your_real_key...
   TICKERS=AAPL,TSLA,NVDA
   FETCH_INTERVAL=15m
   ```

3. Pull the single dependency:

   ```bash
   go mod tidy
   ```

## Run

```bash
go run .
go run . --watch
go run . --json
```

Or build a binary:

```bash
go build -o trade-helper .
./trade-helper        # macOS / Linux
.\trade-helper.exe    # Windows
```

Example output:

```
2026-08-11 19:40:01  trade-helper: scanned 3 ticker(s)
AAPL    FAVORABLE    bullish keywords lead 4 to 1 (upgrade:2, beat:1, strong:1)  (37 articles)
TSLA    UNFAVORABLE  bearish keywords lead 3 to 0 (lawsuit:2, investigation:1)   (28 articles)
NVDA    NEUTRAL      scanned 41 article(s); no bullish or bearish keywords matched (41 articles)
```

Colors are disabled automatically when stdout is not a terminal or when
the `NO_COLOR` environment variable is set.

`--json` (or `OUTPUT_FORMAT=json`) prints one JSON object per scan
instead of the colored table, suitable for cron, bots, or piping.

## Configuration reference

| Variable          | Required | Description                                                            |
| ----------------- | -------- | ---------------------------------------------------------------------- |
| `FINNHUB_API_KEY` | yes      | Your Finnhub API token.                                                |
| `TICKERS`         | yes      | Comma-separated stock symbols, e.g. `AAPL,TSLA,NVDA`.                  |
| `FETCH_INTERVAL`  | no       | Duration (`15m`, `1h`) or bare minutes. Default `15m`. Used by `--watch`. |
| `WATCH`           | no       | `true`/`1` to poll until interrupted. Same as `--watch`. Default off.  |
| `OUTPUT_FORMAT`   | no       | `text` (default) or `json`. Same as `--json`.                          |

Environment variables can be supplied via `.env`, your shell, or your
deployment system. The `.env` file is optional — if it's missing,
`trade-helper` falls back to whatever is already in the environment.

## Sentiment keywords

The classifier matches whole words (case-insensitive) inside each
article's `headline + summary`:

- **Bullish:** `upgrade`, `beat`, `growth`, `strong`, `buy`
- **Bearish:** `downgrade`, `miss`, `layoff`, `investigation`, `lawsuit`, `weak`

This is intentionally crude — it's a placeholder for a real sentiment
model. The signal whose keyword score is higher wins; ties resolve to
`NEUTRAL`.

## Error handling

- Missing `FINNHUB_API_KEY` or `TICKERS` → a clear error and non-zero exit.
- Per-ticker fetch failures (bad symbol, 401/403/429, network) are
  printed inline and the run continues with the remaining tickers.
- If any ticker errored, the process exits with status 1 after printing
  every ticker, so it's safe to use in cron / CI. Watch mode keeps
  polling and exits 0 on Ctrl+C.
- A scan that overruns `FETCH_INTERVAL` causes the next cycle to be skipped.
- Tickers are fetched a few at a time. HTTP 429 responses are retried with backoff.

## Roadmap

- Pluggable sentiment backends
