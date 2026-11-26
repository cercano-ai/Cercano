package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cercano/source/server/pkg/proto"
)

// statsRecentDays bounds the per-day section; totals always cover all tracked usage.
const statsRecentDays = 14

// StatsTimezone returns an IANA zone for calendar buckets, falling back to UTC.
// MetricsBounds rejects "Local", so resolve the viewer's zone the same way the
// Token Metrics page does.
func StatsTimezone() string {
	candidates := []string{os.Getenv("TZ"), time.Local.String()}
	if path, err := filepath.EvalSymlinks("/etc/localtime"); err == nil {
		if i := strings.Index(path, "zoneinfo/"); i >= 0 {
			candidates = append(candidates, path[i+len("zoneinfo/"):])
		}
	}
	for _, name := range candidates {
		name = strings.TrimPrefix(name, ":")
		if name != "" && name != "Local" {
			if _, err := time.LoadLocation(name); err == nil {
				return name
			}
		}
	}
	return "UTC"
}

// StatsRequest is the request behind `cercano stats` and cercano_stats: every
// tracked inference attempt since the accounting cutover, the same population
// the Token Metrics page reports.
func StatsRequest() *proto.GetTokenMetricsRequest {
	return &proto.GetTokenMetricsRequest{Preset: "all", Timezone: StatsTimezone(), Population: "attempts"}
}

// ErrNoAccounting means attempt accounting has not started in this database:
// an agent creates its tables and cutover on first start, so a fresh install
// has nothing to report yet.
var ErrNoAccounting = errors.New("no usage tracked yet: attempt accounting starts when the agent first runs")

// UsageReport queries attempt accounting without a live collector (offline CLI).
// It only reads; it never establishes the cutover itself.
func (s *SQLiteStore) UsageReport(ctx context.Context, now time.Time) (*proto.GetTokenMetricsResponse, error) {
	var since string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM accounting_metadata WHERE key='tracking_since'").Scan(&since)
	if errors.Is(err, sql.ErrNoRows) || (err != nil && strings.Contains(err.Error(), "no such table")) {
		return nil, ErrNoAccounting
	}
	if err != nil {
		return nil, err
	}
	return s.queryTokenMetrics(ctx, StatsRequest(), now, "", nil, true)
}

// UsageReport includes live accounting health for the MCP stats report.
func (c *Collector) UsageReport(ctx context.Context) (*proto.GetTokenMetricsResponse, error) {
	return c.tokenMetrics(ctx, StatsRequest(), true)
}

// queryStatsDays seeks backwards through active calendar days, avoiding both
// widened chart buckets and a scan/query per empty day in sparse history.
// All aggregates share the all-time report's read transaction and population.
func queryStatsDays(ctx context.Context, tx *sql.Tx, base string, args []any, start, end, now time.Time, loc *time.Location) ([]*proto.TokenMetricBucket, error) {
	var days []*proto.TokenMetricBucket
	before := end
	for len(days) < statsRecentDays && before.After(start) {
		var latest sql.NullInt64
		a := append(append([]any{}, args...), before.UnixMicro())
		if err := tx.QueryRowContext(ctx, "SELECT MAX(timestamp) FROM ("+base+") WHERE timestamp<?", a...).Scan(&latest); err != nil {
			return nil, err
		}
		if !latest.Valid {
			break
		}
		local := time.UnixMicro(latest.Int64).In(loc)
		midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
		lower, upper := midnight.UTC(), midnight.AddDate(0, 0, 1).UTC()
		if lower.Before(start) {
			lower = start
		}
		if upper.After(end) {
			upper = end
		}
		a = append(append([]any{}, args...), lower.UnixMicro(), upper.UnixMicro())
		rows, err := tx.QueryContext(ctx, "SELECT "+metricAggregate+" FROM ("+base+") WHERE timestamp>=? AND timestamp<?", a...)
		if err != nil {
			return nil, err
		}
		b := &proto.TokenMetricBucket{StartUnixMicros: lower.UnixMicro(), EndUnixMicros: upper.UnixMicro(), Label: midnight.Format("2006-01-02"), Partial: lower.After(midnight.UTC()) || (lower.Before(now) && upper.After(now))}
		if rows.Next() {
			b.Totals, err = scanMetricTotals(rows, nil)
		}
		rowErr := rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
		if rowErr != nil {
			return nil, rowErr
		}
		days = append(days, b)
		before = lower
	}
	for i, j := 0, len(days)-1; i < j; i, j = i+1, j-1 {
		days[i], days[j] = days[j], days[i]
	}
	return days, nil
}

// FormatTokenMetricsASCII renders a UsageReport as a text dashboard.
// UsageReport totals count the union of missing and unfinished counts in Incomplete.
func FormatTokenMetricsASCII(r *proto.GetTokenMetricsResponse) string {
	var out strings.Builder
	out.WriteString(box("CERCANO USAGE"))
	out.WriteString("\n")

	t := r.GetTotals()
	out.WriteString(sectionHeader("Overview"))
	out.WriteString(fmt.Sprintf("  Tracking since:  %s\n", trackingSince(r)))
	out.WriteString(fmt.Sprintf("  Model calls:     %s\n", formatNumber(int(t.GetRecords()))))
	out.WriteString(fmt.Sprintf("  Input tokens:    %s\n", formatCount(t.GetInput())))
	out.WriteString(fmt.Sprintf("  Output tokens:   %s\n", formatCount(t.GetOutput())))
	if t.GetCacheRead().GetKnownRecords() > 0 || t.GetCacheWrite().GetKnownRecords() > 0 {
		out.WriteString(fmt.Sprintf("  Cache read:      %s (part of input)\n", formatCount(t.GetCacheRead())))
		out.WriteString(fmt.Sprintf("  Cache write:     %s (part of input)\n", formatCount(t.GetCacheWrite())))
	}
	if t.GetReasoning().GetKnownRecords() > 0 {
		out.WriteString(fmt.Sprintf("  Reasoning:       %s (part of output)\n", formatCount(t.GetReasoning())))
	}
	if n := t.GetIncomplete(); n > 0 {
		out.WriteString(fmt.Sprintf("  Incomplete:      %s calls with missing or unfinished counts\n", formatNumber(int(n))))
	}
	out.WriteString("\n")

	for _, d := range []struct{ dim, title string }{{"source", "By Source"}, {"model", "By Model"}, {"provider", "By Provider"}} {
		writeBreakdown(&out, r.GetBreakdowns(), d.dim, d.title)
	}
	if r.GetBreakdownsTruncated() {
		out.WriteString("  (breakdowns truncated to the top 50 values)\n\n")
	}

	var days []*proto.TokenMetricBucket
	for _, b := range r.GetBuckets() {
		if b.GetTotals().GetRecords() > 0 {
			days = append(days, b)
		}
	}
	if len(days) > statsRecentDays {
		days = days[len(days)-statsRecentDays:]
	}
	if len(days) > 0 {
		out.WriteString(sectionHeader(fmt.Sprintf("By Day (%s)", r.GetTimezone())))
		maxTokens := 0
		for _, b := range days {
			maxTokens = max(maxTokens, bucketTokens(b.GetTotals()))
		}
		for _, b := range days {
			total := bucketTokens(b.GetTotals())
			out.WriteString(fmt.Sprintf("  %s %s %s (%d calls)\n", b.GetLabel(), horizontalBar(total, maxTokens), formatNumber(total), b.GetTotals().GetRecords()))
		}
		out.WriteString("\n")
	}

	if h := r.GetHealth(); h != nil && (h.GetCoverageIncomplete() || h.GetLost() > 0 || h.GetUncertain() > 0 || h.GetWriteFailures() > 0) {
		out.WriteString(sectionHeader("Accounting Health"))
		out.WriteString(fmt.Sprintf("  Lost: %d  Uncertain: %d  Write failures: %d  Pending: %d\n", h.GetLost(), h.GetUncertain(), h.GetWriteFailures(), h.GetPending()))
		if h.GetLastError() != "" {
			out.WriteString(fmt.Sprintf("  Last error: %s\n", h.GetLastError()))
		}
		out.WriteString("\n")
	}
	for _, w := range r.GetWarnings() {
		out.WriteString("  Note: " + w + "\n")
	}
	return out.String()
}

// FormatUsageReport renders the attempt report plus, when a host has submitted
// cloud usage (cercano_submit_usage), that host-reported total as a separate
// section. It is a different population and is never added to the totals.
func FormatUsageReport(r *proto.GetTokenMetricsResponse, legacy *Stats) string {
	text := FormatTokenMetricsASCII(r)
	if legacy == nil || legacy.TotalCloudInputTokens+legacy.TotalCloudOutputTokens == 0 {
		return text
	}
	var out strings.Builder
	out.WriteString(text)
	out.WriteString("\n")
	out.WriteString(sectionHeader("Cloud Usage Reported by Host"))
	out.WriteString(fmt.Sprintf("  %s tokens (%s in, %s out), all time\n",
		formatNumber(legacy.TotalCloudInputTokens+legacy.TotalCloudOutputTokens), formatNumber(legacy.TotalCloudInputTokens), formatNumber(legacy.TotalCloudOutputTokens)))
	out.WriteString("  Submitted by the host via cercano_submit_usage; a separate population, not included above.\n")
	return out.String()
}

func writeBreakdown(out *strings.Builder, all []*proto.TokenMetricBreakdown, dim, title string) {
	var rows []*proto.TokenMetricBreakdown
	maxTokens := 0
	for _, b := range all {
		if b.GetDimension() == dim {
			rows = append(rows, b)
			maxTokens = max(maxTokens, bucketTokens(b.GetTotals()))
		}
	}
	if len(rows) == 0 {
		return
	}
	out.WriteString(sectionHeader(title))
	for _, b := range rows {
		name := statsLabel(b.GetValue())
		total := bucketTokens(b.GetTotals())
		out.WriteString(fmt.Sprintf("  %-24s %s %s (%d calls)\n", name, horizontalBar(total, maxTokens), formatNumber(total), b.GetTotals().GetRecords()))
	}
	out.WriteString("\n")
}

// statsLabel keeps breakdown rows aligned: local models can be reported as
// full file paths, so show the file name and cap the width.
func statsLabel(v string) string {
	if v == "" {
		return "(unknown)"
	}
	if i := strings.LastIndexAny(v, `/\`); i >= 0 && i < len(v)-1 {
		v = v[i+1:]
	}
	if r := []rune(v); len(r) > 24 {
		v = string(r[:23]) + "…"
	}
	return v
}

// bucketTokens is reported input plus output; cache and reasoning are subsets.
func bucketTokens(t *proto.TokenMetricTotals) int {
	return int(t.GetInput().GetTokens() + t.GetOutput().GetTokens())
}

func formatCount(c *proto.TokenMetricCount) string {
	if c.GetKnownRecords() == 0 {
		return "unknown"
	}
	return formatNumber(int(c.GetTokens()))
}

func trackingSince(r *proto.GetTokenMetricsResponse) string {
	ts, err := time.Parse(time.RFC3339Nano, r.GetTrackingSince())
	if err != nil {
		return r.GetTrackingSince()
	}
	if loc, err := time.LoadLocation(r.GetTimezone()); err == nil {
		ts = ts.In(loc)
	}
	return ts.Format("2006-01-02 15:04 MST")
}
