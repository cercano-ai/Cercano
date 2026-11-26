package telemetry

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/llm"
	"cercano/source/server/internal/usage"
	"cercano/source/server/pkg/proto"
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

func TestUsageReportLongHistoryActiveDays(t *testing.T) {
	t.Setenv("TZ", "UTC")
	s := metricsStore(t)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := s.db.Exec("UPDATE accounting_metadata SET value=? WHERE key='tracking_since'", start.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	// Jan 1 and Jan 2 used to land in the same widened bucket. Sparse
	// history must return active days, not just the most recent fortnight.
	for i := 0; i < 16; i++ {
		a := accountingFixture(fmt.Sprintf("day-%d", i))
		a.StartedAt = start.AddDate(0, 0, i).Add(time.Hour)
		a.Outcome, a.EndedAt = usage.Completed, a.StartedAt.Add(time.Second)
		a.Tokens = llm.TokenUsage{Input: llm.ReportedTokens(10), Output: llm.ReportedTokens(1), Final: true}
		if err := s.WriteAttempts(t.Context(), []usage.AttemptObservation{a}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := s.UsageReport(t.Context(), start.AddDate(0, 0, 240))
	if err != nil {
		t.Fatal(err)
	}
	if report.GetTotals().GetRecords() != 16 || report.GetTotals().GetInput().GetTokens() != 160 {
		t.Fatalf("all-time totals: %v", report.GetTotals())
	}
	var active []*proto.TokenMetricBucket
	for _, b := range report.GetBuckets() {
		if b.GetTotals().GetRecords() > 0 {
			active = append(active, b)
		}
	}
	if len(active) != 14 {
		t.Fatalf("got %d active day buckets, want 14: %v", len(active), active)
	}
	for i, b := range active {
		want := start.AddDate(0, 0, i+2).Format("2006-01-02")
		if b.GetLabel() != want || b.GetTotals().GetRecords() != 1 || b.GetEndUnixMicros()-b.GetStartUnixMicros() != int64(24*time.Hour/time.Microsecond) {
			t.Errorf("day %d: %v, want %s with one call", i, b, want)
		}
	}
}

func TestUsageReportIncompleteUnion(t *testing.T) {
	s := metricsStore(t)
	a := accountingFixture("started-unknown")
	a.StartedAt = time.Now().Add(-time.Hour)
	a.Outcome, a.EndedAt, a.Tokens = usage.Started, time.Time{}, llm.TokenUsage{}
	if err := s.WriteAttempts(t.Context(), []usage.AttemptObservation{a}); err != nil {
		t.Fatal(err)
	}
	report, err := s.UsageReport(t.Context(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the live-collector path used by MCP as well as offline stats.
	c := NewCollector(s, 10)
	if err := c.EnableAccounting(testAccountingOptions()); err != nil {
		c.Close()
		t.Fatal(err)
	}
	defer c.Close()
	live, err := c.UsageReport(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if live.GetTotals().GetIncomplete() != 1 || len(live.GetBuckets()) != 1 {
		t.Fatalf("live stats: %v", live)
	}
	text := FormatTokenMetricsASCII(report)
	if !strings.Contains(text, "Incomplete:      1 calls") {
		t.Fatalf("one attempt must count once:\n%s", text)
	}
}

// Neither predicate implies the other: even a started observation can carry
// final known counters. Verify the union, not a sum or max of the two counts.
func TestUsageReportIncompleteUnionDisjointAndOverlap(t *testing.T) {
	s := metricsStore(t)
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	for i, kind := range []string{"both", "missing", "unfinished", "complete"} {
		a := accountingFixture(kind)
		a.StartedAt = now.Add(-time.Hour)
		a.Outcome, a.EndedAt = usage.Completed, a.StartedAt.Add(time.Second)
		a.Tokens = llm.TokenUsage{Input: llm.ReportedTokens(1), Output: llm.ReportedTokens(1), Final: true}
		if i == 0 || i == 1 {
			a.Tokens = llm.TokenUsage{}
		}
		if i == 0 || i == 2 {
			a.Outcome, a.EndedAt = usage.Started, time.Time{}
		}
		if err := s.WriteAttempts(t.Context(), []usage.AttemptObservation{a}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := s.UsageReport(t.Context(), now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(FormatTokenMetricsASCII(report), "Incomplete:      3 calls") {
		t.Fatal(FormatTokenMetricsASCII(report))
	}
	shared, err := s.QueryTokenMetrics(t.Context(), StatsRequest(), now, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if shared.GetTotals().GetIncomplete() != 2 || shared.GetTotals().GetUnfinished() != 2 {
		t.Fatalf("shared counters changed: %v", shared.GetTotals())
	}
}

func TestUsageReportCalendarDayDSTAndPartialCutover(t *testing.T) {
	t.Setenv("TZ", "America/New_York")
	s := metricsStore(t)
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	cutover := time.Date(2026, 3, 7, 12, 0, 0, 0, loc)
	if _, err := s.db.Exec("UPDATE accounting_metadata SET value=? WHERE key='tracking_since'", cutover.UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		a := accountingFixture(fmt.Sprintf("dst-%d", i))
		a.StartedAt = cutover.AddDate(0, 0, i)
		if err := s.WriteAttempts(t.Context(), []usage.AttemptObservation{a}); err != nil {
			t.Fatal(err)
		}
	}
	report, err := s.UsageReport(t.Context(), cutover.AddDate(0, 0, 2).Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	days := report.GetBuckets()
	if len(days) != 3 {
		t.Fatalf("days: %v", days)
	}
	if !days[0].GetPartial() || !days[2].GetPartial() || days[1].GetLabel() != "2026-03-08" || days[1].GetEndUnixMicros()-days[1].GetStartUnixMicros() != int64(23*time.Hour/time.Microsecond) {
		t.Fatalf("calendar boundaries/partial flags: %v", days)
	}
}
