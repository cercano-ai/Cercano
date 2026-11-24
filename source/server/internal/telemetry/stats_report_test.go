package telemetry

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/usage"
)

// A tool loop makes several model calls but the legacy events table records one
// summary per turn carrying only the last call's counts. The stats report must
// total every attempt and ignore the legacy row.
func TestUsageReportTotalsEveryToolLoopAttempt(t *testing.T) {
	s := metricsStore(t)
	ctx := t.Context()
	start := time.Now().Add(-time.Hour).UTC()
	var attempts []usage.AttemptObservation
	for i, c := range []struct{ in, out, cache int64 }{{13882, 7253, 0}, {15844, 66, 13878}, {22191, 354, 21898}} {
		a := accountingFixture(string(rune('a' + i)))
		a.Provider, a.Model = "llama_server", "qwen"
		a.StartedAt = start.Add(time.Duration(i) * time.Minute)
		a.EndedAt = a.StartedAt.Add(time.Second)
		a.Outcome = usage.Completed
		a.Tokens = llm.TokenUsage{Input: llm.ReportedTokens(c.in), Output: llm.ReportedTokens(c.out), CacheRead: llm.ReportedTokens(c.cache), Final: true}
		attempts = append(attempts, a)
	}
	recap := accountingFixture("recap")
	recap.Attribution.Source = "recap"
	recap.Provider, recap.Model = "llama_server", "qwen"
	recap.StartedAt = start.Add(5 * time.Minute)
	recap.EndedAt = recap.StartedAt.Add(time.Second)
	recap.Outcome = usage.Completed
	recap.Tokens = llm.TokenUsage{Input: llm.ReportedTokens(139), Output: llm.ReportedTokens(395), Final: true}
	attempts = append(attempts, recap)
	if err := s.WriteAttempts(ctx, attempts); err != nil {
		t.Fatal(err)
	}
	legacy := NewEvent("main", "qwen")
	legacy.InputTokens, legacy.OutputTokens = 22191, 354
	if err := s.RecordEvent(ctx, legacy); err != nil {
		t.Fatal(err)
	}

	report, err := s.UsageReport(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	tot := report.GetTotals()
	if tot.GetRecords() != 4 || tot.GetInput().GetTokens() != 13882+15844+22191+139 || tot.GetOutput().GetTokens() != 7253+66+354+395 || tot.GetCacheRead().GetTokens() != 13878+21898 {
		t.Fatalf("totals %+v", tot)
	}

	text := FormatTokenMetricsASCII(report)
	for _, want := range []string{"Model calls:     4", "Input tokens:    52,056", "Output tokens:   8,068", "Cache read:      35,776 (part of input)", "By Source", "main", "recap", "By Model", "qwen", "By Day"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "Tokens Saved") {
		t.Fatalf("legacy savings estimate leaked into the report:\n%s", text)
	}
}

func TestUsageReportBeforeAccountingStarts(t *testing.T) {
	s, err := NewSQLiteStore(filepath.Join(t.TempDir(), "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if _, err := s.UsageReport(t.Context(), time.Now()); !errors.Is(err, ErrNoAccounting) {
		t.Fatalf("want ErrNoAccounting, got %v", err)
	}
}

func TestFormatTokenMetricsUnknownCounts(t *testing.T) {
	s := metricsStore(t)
	report, err := s.UsageReport(t.Context(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	text := FormatTokenMetricsASCII(report)
	if !strings.Contains(text, "Model calls:     0") || !strings.Contains(text, "Input tokens:    unknown") || strings.Contains(text, "By Day") {
		t.Fatalf("empty report:\n%s", text)
	}
}

func TestStatsLabel(t *testing.T) {
	for in, want := range map[string]string{
		"":     "(unknown)",
		"qwen": "qwen",
		`C:\models\unsloth\Qwen3.8-27B-UD-Q6_K.gguf`:         "Qwen3.8-27B-UD-Q6_K.gguf",
		"/m/a-very-long-model-file-name-that-overflows.gguf": "a-very-long-model-file-…",
		"provider/": "provider/",
	} {
		if got := statsLabel(in); got != want {
			t.Errorf("statsLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStatsTimezoneIsLoadable(t *testing.T) {
	tz := StatsTimezone()
	if tz == "" || tz == "Local" {
		t.Fatalf("timezone %q", tz)
	}
	if _, err := time.LoadLocation(tz); err != nil {
		t.Fatal(err)
	}
}
