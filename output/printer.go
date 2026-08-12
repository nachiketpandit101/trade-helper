// Package output renders analysis results as colored terminal text or JSON.
// Colors are disabled automatically when stdout is not a TTY, when JSON
// mode is on, or when the NO_COLOR environment variable is set
// (see https://no-color.org/).
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/nachi/trade-helper/analysis"
)

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiGreen  = "\x1b[32m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
)

// Printer writes analysis results to an io.Writer.
type Printer struct {
	w      io.Writer
	colors bool
	json   bool
}

// New returns a Printer that writes to w. If w is nil, os.Stdout is used.
// Color output is auto-detected: disabled when NO_COLOR is set, when
// the target writer is not a terminal, or when jsonMode is true.
func New(w io.Writer, jsonMode bool) *Printer {
	if w == nil {
		w = os.Stdout
	}
	return &Printer{
		w:      w,
		json:   jsonMode,
		colors: !jsonMode && shouldUseColor(w),
	}
}

// Report is one scan cycle, used by JSON output.
type Report struct {
	ScannedAt time.Time   `json:"scanned_at"`
	Tickers   []ReportRow `json:"tickers"`
}

// ReportRow is a single ticker in a Report.
type ReportRow struct {
	Ticker       string         `json:"ticker"`
	Signal       string         `json:"signal,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	ArticleCount int            `json:"article_count,omitempty"`
	BullishHits  map[string]int `json:"bullish_hits,omitempty"`
	BearishHits  map[string]int `json:"bearish_hits,omitempty"`
	Error        string         `json:"error,omitempty"`
}

// PrintJSON writes a single JSON document (one line-delimited object).
func (p *Printer) PrintJSON(r Report) error {
	enc := json.NewEncoder(p.w)
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

// JSON reports whether this printer is in JSON mode.
func (p *Printer) JSON() bool {
	return p.json
}

// PrintHeader prints a one-line banner before the per-ticker results.
func (p *Printer) PrintHeader(count int) {
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(p.w, "%s%s  trade-helper: scanned %d ticker(s)%s\n",
		p.style(ansiBold), ts, count, p.style(ansiReset))
}

// PrintResult prints a single ticker's signal and reason.
func (p *Printer) PrintResult(r analysis.Result) {
	color := p.colorFor(r.Signal)
	fmt.Fprintf(p.w, "%s%-6s%s  %s%-11s%s  %s%s%s  %s(%d article%s)%s\n",
		p.style(ansiBold), r.Ticker, p.style(ansiReset),
		color, string(r.Signal), p.style(ansiReset),
		p.style(ansiDim), r.Reason, p.style(ansiReset),
		p.style(ansiDim), r.ArticleCount, pluralS(r.ArticleCount), p.style(ansiReset),
	)
}

// PrintError prints a per-ticker error without bringing down the whole run.
func (p *Printer) PrintError(ticker string, err error) {
	fmt.Fprintf(p.w, "%s%-6s%s  %sERROR%s      %s%s%s\n",
		p.style(ansiBold), strings.ToUpper(ticker), p.style(ansiReset),
		p.style(ansiRed), p.style(ansiReset),
		p.style(ansiDim), err.Error(), p.style(ansiReset),
	)
}

// colorFor returns the ANSI escape for a given signal.
func (p *Printer) colorFor(s analysis.Signal) string {
	switch s {
	case analysis.SignalFavorable:
		return p.style(ansiGreen)
	case analysis.SignalUnfavorable:
		return p.style(ansiRed)
	default:
		return p.style(ansiYellow)
	}
}

// style returns the escape only if colors are enabled; otherwise "".
func (p *Printer) style(code string) string {
	if !p.colors {
		return ""
	}
	return code
}

// shouldUseColor decides whether to emit ANSI codes for the given writer.
func shouldUseColor(w io.Writer) bool {
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	// Character devices (terminals) have ModeCharDevice set.
	return fi.Mode()&os.ModeCharDevice != 0
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
