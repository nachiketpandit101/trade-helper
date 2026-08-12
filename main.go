// Command trade-helper fetches recent company news from Finnhub for the
// configured tickers, runs a simple keyword sentiment scan, and prints
// a per-ticker BUY/SELL/HOLD-style signal to the terminal.
//
// The default is a single scan. Pass --watch (or set WATCH=true) to poll
// on FetchInterval until interrupted.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
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
	watchFlag := flag.Bool("watch", false, "poll on FETCH_INTERVAL until interrupted")
	flag.Parse()

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
	if *watchFlag {
		cfg.Watch = true
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fetcher := news.New(cfg.APIKey)
	printer := output.New(os.Stdout)

	scan := func() error {
		scanCtx, cancelScan := context.WithTimeout(ctx, 30*time.Second)
		defer cancelScan()
		return scanTickers(scanCtx, cfg, fetcher, printer)
	}

	if err := scan(); !cfg.Watch {
		return err
	}

	fmt.Fprintf(os.Stderr, "trade-helper: watching every %s (Ctrl+C to stop)\n", cfg.FetchInterval)

	ticks := time.NewTicker(cfg.FetchInterval)
	defer ticks.Stop()

	var busy atomic.Bool
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticks.C:
			if !busy.CompareAndSwap(false, true) {
				fmt.Fprintf(os.Stderr, "trade-helper: skipping cycle; previous scan still running\n")
				continue
			}
			go func() {
				defer busy.Store(false)
				_ = scan()
			}()
		}
	}
}

func scanTickers(ctx context.Context, cfg *config.Config, fetcher *news.Fetcher, printer *output.Printer) error {
	printer.PrintHeader(len(cfg.Tickers))

	hadError := false
	for _, ticker := range cfg.Tickers {
		articles, err := fetcher.FetchCompanyNews(ctx, ticker)
		if err != nil {
			printer.PrintError(ticker, err)
			hadError = true
			continue
		}
		printer.PrintResult(analysis.Analyze(ticker, articles))
	}

	if hadError {
		return errors.New("one or more tickers failed; see output above")
	}
	return nil
}
