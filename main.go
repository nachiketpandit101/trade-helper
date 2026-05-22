// Command trade-helper fetches recent company news from Finnhub for the
// configured tickers, runs a simple keyword sentiment scan, and prints
// a per-ticker BUY/SELL/HOLD-style signal to the terminal.
//
// This build runs once and exits. A polling loop on FetchInterval is
// planned for a follow-up.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/nachi/trade-helper/analysis"
	"github.com/nachi/trade-helper/config"
	"github.com/nachi/trade-helper/news"
	"github.com/nachi/trade-helper/output"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "trade-helper: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// .env is optional -- a missing file is fine as long as the env vars
	// are set some other way (CI, shell export, etc.). Any other load
	// error (malformed file, permission denied) should be surfaced.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(os.Stderr, "trade-helper: warning: could not load .env: %v\n", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Give the whole run a hard ceiling so a hung HTTP call can't pin the CLI.
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	defer cancelTimeout()

	fetcher := news.New(cfg.APIKey)
	printer := output.New(os.Stdout)
	printer.PrintHeader(len(cfg.Tickers))

	hadError := false
	for _, ticker := range cfg.Tickers {
		articles, err := fetcher.FetchCompanyNews(ctx, ticker)
		if err != nil {
			printer.PrintError(ticker, err)
			hadError = true
			continue
		}
		result := analysis.Analyze(ticker, articles)
		printer.PrintResult(result)
	}

	if hadError {
		return errors.New("one or more tickers failed; see output above")
	}
	return nil
}
