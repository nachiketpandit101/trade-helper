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
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"github.com/nachi/trade-helper/analysis"
	"github.com/nachi/trade-helper/config"
	"github.com/nachi/trade-helper/news"
	"github.com/nachi/trade-helper/output"
)

const maxConcurrentFetches = 3

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "trade-helper: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	watchFlag := flag.Bool("watch", false, "poll on FETCH_INTERVAL until interrupted")
	jsonFlag := flag.Bool("json", false, "print JSON instead of colored text")
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
	if *jsonFlag {
		cfg.JSON = true
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fetcher := news.New(cfg.APIKey)
	printer := output.New(os.Stdout, cfg.JSON)

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
	if !printer.JSON() {
		printer.PrintHeader(len(cfg.Tickers))
	}

	type outcome struct {
		ticker string
		result analysis.Result
		err    error
	}
	outcomes := make([]outcome, len(cfg.Tickers))
	sem := make(chan struct{}, maxConcurrentFetches)
	var wg sync.WaitGroup

	for i, ticker := range cfg.Tickers {
		i, ticker := i, ticker
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			articles, err := fetcher.FetchCompanyNews(ctx, ticker)
			if err != nil {
				outcomes[i] = outcome{ticker: ticker, err: err}
				return
			}
			outcomes[i] = outcome{ticker: ticker, result: analysis.Analyze(ticker, articles)}
		}()
	}
	wg.Wait()

	hadError := false
	if printer.JSON() {
		rep := output.Report{ScannedAt: time.Now(), Tickers: make([]output.ReportRow, 0, len(outcomes))}
		for _, o := range outcomes {
			if o.err != nil {
				rep.Tickers = append(rep.Tickers, output.ReportRow{Ticker: o.ticker, Error: o.err.Error()})
				hadError = true
				continue
			}
			rep.Tickers = append(rep.Tickers, output.ReportRow{
				Ticker:       o.result.Ticker,
				Signal:       string(o.result.Signal),
				Reason:       o.result.Reason,
				ArticleCount: o.result.ArticleCount,
				BullishHits:  o.result.BullishHits,
				BearishHits:  o.result.BearishHits,
			})
		}
		if err := printer.PrintJSON(rep); err != nil {
			return err
		}
	} else {
		for _, o := range outcomes {
			if o.err != nil {
				printer.PrintError(o.ticker, o.err)
				hadError = true
				continue
			}
			printer.PrintResult(o.result)
		}
	}

	if hadError {
		return errors.New("one or more tickers failed; see output above")
	}
	return nil
}
