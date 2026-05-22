// Package analysis turns a slice of news articles into a coarse trade
// signal using simple keyword matching. This is intentionally naive --
// it's a placeholder for a real sentiment model.
package analysis

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nachi/trade-helper/news"
)

// Signal is the high-level recommendation surfaced to the user.
type Signal string

const (
	SignalFavorable   Signal = "FAVORABLE"
	SignalUnfavorable Signal = "UNFAVORABLE"
	SignalNeutral     Signal = "NEUTRAL"
)

// Result is the analysis output for a single ticker.
type Result struct {
	Ticker       string
	Signal       Signal
	Reason       string
	ArticleCount int
	BullishHits  map[string]int
	BearishHits  map[string]int
}

// bullishKeywords / bearishKeywords are matched as whole words against
// the lower-cased article text. Order is preserved so the "reason"
// string is deterministic for the same input.
var (
	bullishKeywords = []string{"upgrade", "beat", "growth", "strong", "buy"}
	bearishKeywords = []string{"downgrade", "miss", "layoff", "investigation", "lawsuit", "weak"}
)

// Analyze runs the keyword scan over the supplied articles for a ticker.
// It is safe to call with an empty slice -- the result will be NEUTRAL
// with an explanatory reason.
func Analyze(ticker string, articles []news.Article) Result {
	r := Result{
		Ticker:       strings.ToUpper(strings.TrimSpace(ticker)),
		ArticleCount: len(articles),
		BullishHits:  map[string]int{},
		BearishHits:  map[string]int{},
	}

	if len(articles) == 0 {
		r.Signal = SignalNeutral
		r.Reason = "no recent news articles found"
		return r
	}

	for _, a := range articles {
		text := strings.ToLower(a.Headline + " " + a.Summary)
		for _, kw := range bullishKeywords {
			if containsWord(text, kw) {
				r.BullishHits[kw]++
			}
		}
		for _, kw := range bearishKeywords {
			if containsWord(text, kw) {
				r.BearishHits[kw]++
			}
		}
	}

	bullScore := sumCounts(r.BullishHits)
	bearScore := sumCounts(r.BearishHits)

	switch {
	case bullScore == 0 && bearScore == 0:
		r.Signal = SignalNeutral
		r.Reason = fmt.Sprintf("scanned %d article(s); no bullish or bearish keywords matched", r.ArticleCount)
	case bullScore > bearScore:
		r.Signal = SignalFavorable
		r.Reason = fmt.Sprintf("bullish keywords lead %d to %d (%s)",
			bullScore, bearScore, formatHits(r.BullishHits))
	case bearScore > bullScore:
		r.Signal = SignalUnfavorable
		r.Reason = fmt.Sprintf("bearish keywords lead %d to %d (%s)",
			bearScore, bullScore, formatHits(r.BearishHits))
	default:
		r.Signal = SignalNeutral
		r.Reason = fmt.Sprintf("bullish and bearish keywords tied at %d each", bullScore)
	}

	return r
}

// containsWord reports whether `word` appears in `text` surrounded by
// non-letter boundaries. This avoids matching e.g. "strongarm" when
// looking for "strong". Both inputs must already be lower-cased.
func containsWord(text, word string) bool {
	if word == "" {
		return false
	}
	idx := 0
	for {
		off := strings.Index(text[idx:], word)
		if off < 0 {
			return false
		}
		start := idx + off
		end := start + len(word)
		if isWordBoundary(text, start-1) && isWordBoundary(text, end) {
			return true
		}
		idx = start + 1
		if idx >= len(text) {
			return false
		}
	}
}

// isWordBoundary returns true when position `i` is outside the string
// or sits on a non-letter, non-digit byte. Good enough for ASCII
// English headlines from a news API.
func isWordBoundary(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return true
	}
	c := s[i]
	switch {
	case c >= 'a' && c <= 'z':
		return false
	case c >= '0' && c <= '9':
		return false
	default:
		return true
	}
}

func sumCounts(m map[string]int) int {
	total := 0
	for _, v := range m {
		total += v
	}
	return total
}

// formatHits renders a deterministic "kw:count, kw:count" string from
// the keyword map, sorted by count (desc) then keyword (asc).
func formatHits(m map[string]int) string {
	if len(m) == 0 {
		return "none"
	}
	type kv struct {
		k string
		v int
	}
	pairs := make([]kv, 0, len(m))
	for k, v := range m {
		if v > 0 {
			pairs = append(pairs, kv{k, v})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].v != pairs[j].v {
			return pairs[i].v > pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, fmt.Sprintf("%s:%d", p.k, p.v))
	}
	return strings.Join(parts, ", ")
}
