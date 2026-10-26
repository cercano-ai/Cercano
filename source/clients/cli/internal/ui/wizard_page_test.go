package ui

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"cercano/source/clients/cli/internal/theme"
	"cercano/source/clients/cli/internal/wizard"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

func newTestWizardPage(t *testing.T) *wizardPage {
	t.Helper()
	t.Setenv("CERCANO_WIZARD_STATE", filepath.Join(t.TempDir(), "wizard_state.yaml"))
	p := theme.BuiltinThemes()[0]
	wp := newWizardPage(nil, p.Palette, theme.NewStyles(p.Palette), 100, 40)
	// With a nil agent loadProviders() is a no-op, so seed the provider catalog
	// the cloud step renders. Mirrors the agent catalog's order/tiers (anthropic,
	// openai (responses), openai, …) so the step-navigation presses below land on
	// the providers the assertions expect.
	// Legacy navigation fixtures continue to exercise scoped apply/edit primitives.
	// Resource-first entry and migration are tested through the production constructor.
	wp.state.FlowVersion = 0
	wp.state.Step = wizard.StepLocus
	wp.providers = wizardTestProviders()
	// Existing navigation tests exercise the optional customization path.
	wp.customizingLocus = true
	wp.cursor = wp.defaultCursor()
	return wp
}

// wizardTestProviders is the fixture catalog the wizard tests navigate. Only ID,
// Label, and Tier matter here: the eager commit path (commitKeyFn/commitMeridianFn)
// is stubbed in every test, so flavor/backend/base-URL are irrelevant to them.
func wizardTestProviders() []agentclient.CloudProvider {
	return []agentclient.CloudProvider{
		{ID: "anthropic", Label: "anthropic", Tier: "verified"},
		{ID: "openai-responses", Label: "openai (responses)", Tier: "untested"},
		{ID: "openai", Label: "openai", Tier: "untested"},
		{ID: "gemini", Label: "gemini", Tier: "verified"},
		{ID: "groq", Label: "groq", Tier: "untested"},
		{ID: "deepinfra", Label: "deepinfra", Tier: "untested"},
		{ID: "together", Label: "together", Tier: "untested"},
		{ID: "openrouter", Label: "openrouter", Tier: "untested"},
		{ID: "deepseek", Label: "deepseek", Tier: "untested"},
		{ID: "bedrock", Label: "bedrock", Tier: "coming_soon"},
	}
}

func press(t *testing.T, wp *wizardPage, code rune) bool {
	t.Helper()
	_, closed := wp.Update(tea.KeyPressMsg{Code: code})
	return closed
}

func TestWizardCloudPathEndToEnd(t *testing.T) {
	wp := newTestWizardPage(t)

	// Step 1 (locus, first): the cursor sits on the recommended cloud_primary.
	rows := wp.rows()
	if rows[wp.cursor].Key != "cloud_primary" {
		t.Fatalf("locus cursor: want cloud_primary, got %s", rows[wp.cursor].Key)
	}
	press(t, wp, tea.KeyEnter)
	if wp.state.Step != wizard.StepCloud {
		t.Fatalf("after locus: want %s, got %s", wizard.StepCloud, wp.state.Step)
	}

	// A connected account is discovered when the shared provider popover
	// returns. The root-model tests cover opening and OAuth dispatch.
	wp.curActiveProfile = "claude"
	wp.curProfiles = []agentclient.CloudProfileInfo{{Name: "claude", Provider: "anthropic"}}
	press(t, wp, tea.KeyEnter)
	if wp.state.Step != wizard.StepOpen || wp.state.AuthMethod != "existing" {
		t.Fatalf("connected account did not continue: %+v", wp.state)
	}

	// Step 3 (open): autofill should have filled both sides for every tier.
	if wp.state.TierPicks["most_capable.cloud"] == "" {
		t.Error("autofill: most_capable.cloud empty")
	}
	if wp.state.TierPicks["llama_server.fast_light_text"] == "" {
		t.Error("autofill: llama_server.fast_light_text empty")
	}
	// Continue is the last row.
	for range wp.rows() {
		press(t, wp, tea.KeyDown)
	}
	press(t, wp, tea.KeyEnter)
	if wp.state.Step != wizard.StepDone {
		t.Fatalf("after tiers: want %s, got %s", wizard.StepDone, wp.state.Step)
	}

	// Finish applies the config, clears the resume file, closes the page.
	applied := false
	wp.applyFn = func() error { applied = true; return nil }
	if closed := press(t, wp, tea.KeyEnter); !closed {
		t.Fatal("finish: want page closed")
	}
	if !applied {
		t.Error("finish: applyFn not called")
	}
	if _, ok := wizard.Load(); ok {
		t.Error("finish: resume state should be cleared")
	}
}

func TestWizardApplyFailureKeepsState(t *testing.T) {
	wp := newTestWizardPage(t)
	press(t, wp, tea.KeyDown)
	press(t, wp, tea.KeyEnter) // open (second row)
	for range wp.rows() {
		press(t, wp, tea.KeyDown)
	}
	press(t, wp, tea.KeyEnter) // continue → done

	wp.applyFn = func() error { return fmt.Errorf("agent unreachable") }
	if closed := press(t, wp, tea.KeyEnter); closed {
		t.Fatal("apply failure: page must stay open")
	}
	if !strings.Contains(wp.status, "apply failed") {
		t.Errorf("apply failure: status %q should surface the error", wp.status)
	}
	if _, ok := wizard.Load(); !ok {
		t.Error("apply failure: resume state must be preserved for retry")
	}

	// Retry after the failure succeeds and closes.
	wp.applyFn = func() error { return nil }
	if closed := press(t, wp, tea.KeyEnter); !closed {
		t.Fatal("retry: want page closed")
	}
}

func TestWizardOpenPathSkipsCloud(t *testing.T) {
	wp := newTestWizardPage(t)
	press(t, wp, tea.KeyDown)
	press(t, wp, tea.KeyEnter) // open (second row)
	if wp.state.Step != wizard.StepOpen {
		t.Fatalf("open primary: want %s, got %s", wizard.StepOpen, wp.state.Step)
	}
	// Only open-side tier rows should appear later.
	for _, r := range wp.rows() {
		if strings.HasSuffix(r.Key, "."+wizard.SideCloud) {
			t.Errorf("open path: unexpected cloud tier row %s", r.Key)
		}
	}
}

func TestWizardEscResumesMidRun(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "wizard_state.yaml")
	t.Setenv("CERCANO_WIZARD_STATE", statePath)
	p := theme.BuiltinThemes()[0]
	wp := newWizardPage(nil, p.Palette, theme.NewStyles(p.Palette), 100, 40)

	pressRows(t, wp, "resource_next") // providers → configuration choices
	if closed := press(t, wp, tea.KeyEscape); closed {
		t.Fatal("esc on cloud step should go back, not close")
	}
	if wp.state.Step != wizard.StepResources {
		t.Fatalf("esc: want %s, got %s", wizard.StepResources, wp.state.Step)
	}
	if closed := press(t, wp, tea.KeyEscape); !closed {
		t.Fatal("esc on first step should close the page")
	}

	// A fresh page resumes from the persisted state.
	wp2 := newWizardPage(nil, p.Palette, theme.NewStyles(p.Palette), 100, 40)
	if wp2.state.Step != wizard.StepResources {
		t.Fatalf("resume: want %s, got %s", wizard.StepResources, wp2.state.Step)
	}
	if wp2.state.LocusMode != "" || wp2.state.ResourcePage != 0 {
		t.Fatalf("resource questions must not invent a routing preference: %+v", wp2.state)
	}
}

func TestWizardTierPickerRecordsPick(t *testing.T) {
	wp := newTestWizardPage(t)
	press(t, wp, tea.KeyDown)  // cloud_primary → open_only
	press(t, wp, tea.KeyEnter) // open_only → open step (skips cloud)

	// Enter on the first tier row opens the picker.
	press(t, wp, tea.KeyEnter)
	if wp.picker == nil {
		t.Fatal("enter on tier row: want picker open")
	}
	// First candidate is the recommendation, already the autofilled pick.
	rows := wp.tierPickerCandidates("llama_server.most_capable")
	if len(rows) < 2 {
		t.Fatalf("picker: want recommendation + clear rows, got %d", len(rows))
	}
	if !rows[0].Selected {
		t.Errorf("first candidate: want Selected=true (autofilled pick), got false; hint=%q", rows[0].Hint)
	}
	if rows[len(rows)-1].Key != "-" {
		t.Errorf("last row: want clear row, got %q", rows[len(rows)-1].Key)
	}

	// Down to the clear row, select: pick removed, picker closed, persisted.
	for range rows {
		press(t, wp, tea.KeyDown)
	}
	press(t, wp, tea.KeyEnter)
	if wp.picker != nil {
		t.Fatal("select: want picker closed")
	}
	if _, ok := wp.state.TierPicks["llama_server.most_capable"]; ok {
		t.Error("clear: pick should be removed")
	}
	st, ok := wizard.Load()
	if !ok {
		t.Fatal("want persisted state")
	}
	if _, exists := st.TierPicks["llama_server.most_capable"]; exists {
		t.Error("clear: persisted state should not carry the removed pick")
	}

	// Reopen and pick the recommendation again.
	press(t, wp, tea.KeyEnter)
	press(t, wp, tea.KeyEnter)
	if wp.state.TierPicks["llama_server.most_capable"] == "" {
		t.Error("pick: want slot filled from picker selection")
	}
}

func TestWizardPickerEscClosesWithoutChange(t *testing.T) {
	wp := newTestWizardPage(t)
	press(t, wp, tea.KeyDown)
	press(t, wp, tea.KeyEnter) // open (second row)
	before := wp.state.TierPicks["llama_server.most_capable"]
	press(t, wp, tea.KeyEnter) // open picker
	press(t, wp, tea.KeyEscape)
	if wp.picker != nil {
		t.Fatal("esc: want picker closed")
	}
	if wp.state.TierPicks["llama_server.most_capable"] != before {
		t.Error("esc: pick must be unchanged")
	}
	if wp.state.Step != wizard.StepOpen {
		t.Errorf("esc: page must stay on tiers, got %s", wp.state.Step)
	}
}

func TestWizardAPIKeyFlow(t *testing.T) {
	wp := newTestWizardPage(t)
	// Exercise the retained legacy key-entry primitive directly; new setup
	// uses the shared settings editor (covered by provider-popover tests).
	wp.state.Step = wizard.StepCloud
	wp.state.LocusMode = "cloud_primary"
	wp.state.CloudProvider = "openai"
	wp.authPick = true
	wp.cursor = 0

	press(t, wp, tea.KeyDown)
	press(t, wp, tea.KeyEnter) // api_key (second auth row)
	if !wp.keyEntry {
		t.Fatal("api_key: want key prompt open")
	}

	// Empty key: prompt, no commit.
	press(t, wp, tea.KeyEnter)
	if !strings.Contains(wp.status, "enter a key") {
		t.Errorf("empty key: status %q", wp.status)
	}

	// Failing commit keeps the prompt.
	wp.commitKeyFn = func(string) error { return fmt.Errorf("boom") }
	wp.keyInput.SetValue("sk-test-123")
	press(t, wp, tea.KeyEnter)
	if !wp.keyEntry || !strings.Contains(wp.status, "key setup failed") {
		t.Fatalf("failed commit: want prompt kept + error status, got keyEntry=%v status=%q", wp.keyEntry, wp.status)
	}

	// Successful commit records the key agent-side and advances.
	var got string
	wp.commitKeyFn = func(k string) error { got = k; return nil }
	press(t, wp, tea.KeyEnter)
	if got != "sk-test-123" {
		t.Errorf("commit: want key passed through, got %q", got)
	}
	if wp.state.Step != wizard.StepOpen {
		t.Fatalf("after key: want %s, got %s", wizard.StepOpen, wp.state.Step)
	}

	// The key must never be persisted in wizard state.
	st, ok := wizard.Load()
	if !ok {
		t.Fatal("want persisted state")
	}
	if strings.Contains(fmt.Sprintf("%+v", st), "sk-test-123") {
		t.Error("resume state must not contain the API key")
	}
}

func TestWizardKeyEntryEscReturnsToAuthPick(t *testing.T) {
	wp := newTestWizardPage(t)
	wp.state.Step = wizard.StepCloud
	wp.state.LocusMode = "cloud_primary"
	wp.state.CloudProvider = "anthropic"
	wp.authPick = true
	wp.cursor = 0
	press(t, wp, tea.KeyDown)
	press(t, wp, tea.KeyEnter) // api_key
	if !wp.keyEntry {
		t.Fatal("want key prompt")
	}
	press(t, wp, tea.KeyEscape)
	if wp.keyEntry {
		t.Fatal("esc: want prompt closed")
	}
	if !wp.authPick {
		t.Fatal("esc: want auth-method screen back")
	}
}

func TestWizardCloudChoicesKeepRecommendationsInherited(t *testing.T) {
	state := wizard.State{LocusMode: "cloud_primary", TierPicks: map[string]string{"most_capable.cloud": "recommended", "everyday.cloud": "custom"}}
	before := &agentclient.CloudModelChoices{ImageModel: "image", TierOverrides: map[string]string{"economy": "existing"}}
	if choices, err := wizardCloudChoices(state, before); err != nil || choices != nil {
		t.Fatal("autofill became overrides")
	}
	state.CloudPicksEdited = map[string]bool{"everyday.cloud": true}
	choices, err := wizardCloudChoices(state, before)
	if err != nil {
		t.Fatal(err)
	}
	if choices.TierOverrides["standard"] != "custom" || choices.TierOverrides["premium"] != "" || choices.TierOverrides["economy"] != "existing" || choices.ImageModel != "image" {
		t.Fatalf("quality mapping lost choices: %+v", choices)
	}
	if len(before.TierOverrides) != 1 {
		t.Fatal("wizard modified loaded profile")
	}
}

func TestWizardFinishUpdateKeepsCloudChoicesSeparate(t *testing.T) {
	// Cloud quality choices use the profile mutation, not the general config patch.
	u := wizardFinishUpdate(wizard.State{
		CloudProvider: "anthropic",
		LocusMode:     "cloud_primary",
		TierPicks:     map[string]string{"everyday.cloud": "claude-opus-5"},
	})
	if u.LocusMode != "cloud_primary" {
		t.Fatalf("locus patch wrong: %+v", u)
	}
	if u.CloudModel != "" {
		t.Fatalf("finish must not write retired CloudModel, got %q", u.CloudModel)
	}
	// Open path: no cloud provider configured → no CloudModel patch (the
	// wantCloudRebuild branch errors without an active profile).
	u2 := wizardFinishUpdate(wizard.State{LocusMode: "open_primary"})
	if u2.CloudModel != "" {
		t.Fatalf("open path must not patch CloudModel, got %q", u2.CloudModel)
	}
}

func pressQ(t *testing.T, wp *wizardPage) bool {
	t.Helper()
	_, closed := wp.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	return closed
}

func TestWizardAbandonTrapdoor(t *testing.T) {
	wp := newTestWizardPage(t)
	press(t, wp, tea.KeyEnter) // cloud (first row) — state now persisted mid-run

	// First q only arms and asks.
	if closed := pressQ(t, wp); closed {
		t.Fatal("first q: page must stay open")
	}
	if !wp.abandonArmed || !strings.Contains(wp.status, "abandon setup?") {
		t.Fatalf("first q: want armed + confirm status, got armed=%v status=%q", wp.abandonArmed, wp.status)
	}

	// Second q clears the resume file and closes.
	if closed := pressQ(t, wp); !closed {
		t.Fatal("second q: want page closed")
	}
	if _, ok := wizard.Load(); ok {
		t.Error("abandon: resume state should be cleared")
	}
}

func TestWizardAbandonDisarmsOnOtherKey(t *testing.T) {
	wp := newTestWizardPage(t)

	pressQ(t, wp)
	press(t, wp, tea.KeyDown) // any other key disarms
	if wp.abandonArmed {
		t.Fatal("other key: want disarmed")
	}
	if wp.status != "" {
		t.Errorf("other key: confirm status should clear, got %q", wp.status)
	}
	// The next q starts the confirm over instead of closing.
	if closed := pressQ(t, wp); closed {
		t.Fatal("q after disarm: must arm again, not close")
	}
}

func TestWizardAbandonClearFailureStaysOpen(t *testing.T) {
	wp := newTestWizardPage(t)
	press(t, wp, tea.KeyEnter) // cloud (first row)

	// Simulate a Clear failure by temporarily making the Clear function return an error
	wp.clearFn = func() error { return fmt.Errorf("state write failed") }

	pressQ(t, wp)
	if closed := pressQ(t, wp); closed {
		t.Fatal("clear failure: page must stay open")
	}
	if !strings.Contains(wp.status, "could not clear wizard state") {
		t.Errorf("clear failure: status %q should surface the error", wp.status)
	}
	if _, ok := wizard.Load(); !ok {
		t.Error("clear failure: resume state must be preserved for retry")
	}

	// Retry after the failure succeeds and closes.
	wp.clearFn = nil
	pressQ(t, wp)
	if closed := pressQ(t, wp); !closed {
		t.Fatal("retry: want page closed")
	}
}

func TestWizardKeyEntryQTypesIntoPrompt(t *testing.T) {
	wp := newTestWizardPage(t)
	wp.state.Step = wizard.StepCloud
	wp.state.LocusMode = "cloud_primary"
	wp.state.CloudProvider = "anthropic"
	wp.authPick = true
	wp.cursor = 0
	press(t, wp, tea.KeyDown)
	press(t, wp, tea.KeyEnter) // api_key → masked prompt
	if !wp.keyEntry {
		t.Fatal("want key prompt open")
	}
	if closed := pressQ(t, wp); closed {
		t.Fatal("q in key prompt: must not close the page")
	}
	if wp.abandonArmed {
		t.Fatal("q in key prompt: must not arm abandon")
	}
	if got := wp.keyInput.Value(); got != "q" {
		t.Errorf("q in key prompt: want it typed into the input, got %q", got)
	}
}

func TestWizardViewRendersTierPurposes(t *testing.T) {
	wp := newTestWizardPage(t)
	wp.state.LocusMode = "open_only"
	wp.state.Step = wizard.StepOpen
	wp.autofillTiers()
	v := wp.View()
	for _, want := range []string{"most-capable", "easy to change later", "step 2 of 2"} {
		if !strings.Contains(v, want) {
			t.Errorf("tiers view: missing %q", want)
		}
	}
}

func TestWizardOpenAutofillUsesCatalog(t *testing.T) {
	wp := newTestWizardPage(t)
	wp.state.LocusMode = "open_only"
	wp.catalog = agentclient.RuntimeModelCatalog{
		Models: []agentclient.RuntimeModel{
			{ID: "llama_server:catalog:qwen3-14b-q4_k_m", DisplayName: "Qwen3-14B Q4_K_M", Runtime: "llama_server", Source: "catalog"},
			{ID: "llama_server:catalog:phi-4-mini-instruct-q4_k_m", DisplayName: "Phi-4-mini Instruct Q4_K_M", Runtime: "llama_server", Source: "catalog"},
			{ID: "llama_server:catalog:nomic-embed-text-v1.5-f16", DisplayName: "Nomic Embed Text v1.5 f16", Runtime: "llama_server", Source: "catalog"},
		},
		RecommendedOpenModels: map[string]string{
			"most_capable":    "llama_server:catalog:qwen3-14b-q4_k_m",
			"everyday":        "llama_server:catalog:qwen3-14b-q4_k_m",
			"fast_light":      "llama_server:catalog:phi-4-mini-instruct-q4_k_m",
			"fast_light_text": "llama_server:catalog:phi-4-mini-instruct-q4_k_m",
			"embedding":       "llama_server:catalog:nomic-embed-text-v1.5-f16",
		},
	}
	wp.catalogOK = true
	wp.state.TierPicks = nil

	wp.autofillTiers()

	// Open slots must hold the RAM-tiered curated display names, not the shipped
	// open recs (which recommend the gate-incompatible qwen3-coder-next).
	if got := wp.state.TierPicks["llama_server.most_capable"]; got != "Qwen3-14B Q4_K_M" {
		t.Fatalf("llama_server.most_capable: want curated display name, got %q", got)
	}
	if got := wp.state.TierPicks["llama_server.fast_light"]; got != "Phi-4-mini Instruct Q4_K_M" {
		t.Fatalf("llama_server.fast_light: want curated display name, got %q", got)
	}
	// The embedding slot isn't in wizardTierOrder; autofillTiers must still fill
	// it from the catalog recommendation, else the embedding row shows "—".
	if got := wp.state.TierPicks["llama_server.embedding"]; got != "Nomic Embed Text v1.5 f16" {
		t.Fatalf("llama_server.embedding: want the curated embedding display name, got %q", got)
	}
	if strings.Contains(fmt.Sprintf("%v", wp.state.TierPicks), "qwen3-coder-next") {
		t.Errorf("open picks must not carry the incompatible recs model: %v", wp.state.TierPicks)
	}
}

func TestWizardEnrollOpenDownloads(t *testing.T) {
	wp := newTestWizardPage(t)
	wp.catalog = agentclient.RuntimeModelCatalog{
		Models: []agentclient.RuntimeModel{
			{ID: "llama_server:catalog:qwen3-14b-q4_k_m", DisplayName: "Qwen3-14B Q4_K_M", Runtime: "llama_server", Source: "catalog", DownloadState: "not_downloaded"},
			{ID: "llama_server:catalog:phi-4-mini-instruct-q4_k_m", DisplayName: "Phi-4-mini Instruct Q4_K_M", Runtime: "llama_server", Source: "catalog", DownloadState: "downloaded"},
		},
	}
	wp.catalogOK = true
	wp.state.TierPicks = map[string]string{
		"llama_server.most_capable": "Qwen3-14B Q4_K_M",
		"llama_server.everyday":     "Qwen3-14B Q4_K_M",           // duplicate of most_capable -> one download
		"llama_server.fast_light":   "Phi-4-mini Instruct Q4_K_M", // already downloaded -> skipped
		"most_capable.cloud":        "claude-opus-5",              // cloud slot -> ignored
	}
	var got []string
	wp.downloadFn = func(_ context.Context, runtime, modelID string) error {
		if runtime != "llama_server" {
			t.Errorf("runtime: want llama_server, got %q", runtime)
		}
		got = append(got, modelID)
		return nil
	}

	wp.enrollOpenDownloads(context.Background())

	// Exactly the distinct, not-yet-downloaded curated id, resolved from its
	// display name: duplicates deduped, downloaded skipped, cloud ignored.
	if len(got) != 1 || got[0] != "llama_server:catalog:qwen3-14b-q4_k_m" {
		t.Fatalf("want the single distinct not-downloaded curated id, got %v", got)
	}
}

func TestWrapWords(t *testing.T) {
	// The bug: a long annotation ran off the right edge instead of wrapping.
	// wrapWords must break on spaces, never exceed width, and preserve every
	// word in order.
	long := "highest-quality frontier model in the cloud; open co-processor for background work (recommended)"
	for _, width := range []int{10, 20, 40, 89} {
		lines := wrapWords(long, width)
		if len(lines) < 2 {
			t.Errorf("width %d: expected the long annotation to wrap, got %d line(s)", width, len(lines))
		}
		for _, ln := range lines {
			// A single word longer than width is allowed to overflow on its
			// own line; only reject a line that packed more than it should.
			if n := len([]rune(ln)); n > width && strings.Contains(strings.TrimSpace(ln), " ") {
				if firstWordFits(ln, width) {
					t.Errorf("width %d: line %q exceeds width %d", width, ln, width)
				}
			}
		}
		if got := strings.Join(lines, " "); got != long {
			t.Errorf("width %d: words not preserved:\n got %q\nwant %q", width, got, long)
		}
	}

	// width < 1 yields the whole string on one line (fallback, no panic).
	if got := wrapWords(long, 0); len(got) != 1 || got[0] != long {
		t.Errorf("width 0: want single unbroken line, got %v", got)
	}
	// A word longer than width stands alone rather than being split.
	if got := wrapWords("supercalifragilistic ok", 5); len(got) != 2 || got[0] != "supercalifragilistic" {
		t.Errorf("oversized word: want it alone on its line, got %v", got)
	}
	if got := wrapWords("", 10); got != nil {
		t.Errorf("empty: want nil, got %v", got)
	}
}

// firstWordFits reports whether line's first word fits within width, i.e.
// the line could legitimately have been packed tighter.
func firstWordFits(line string, width int) bool {
	fields := strings.Fields(line)
	return len(fields) > 0 && len([]rune(fields[0])) <= width
}

func TestWizardFinishDoesNotWriteRetiredCloudModel(t *testing.T) {
	update := wizardFinishUpdate(wizard.State{LocusMode: "cloud_only", CloudProvider: "openai", TierPicks: map[string]string{"everyday.cloud": "selected"}})
	if update.CloudModel != "" {
		t.Fatalf("wizard still writes retired cloud_model=%q", update.CloudModel)
	}
}

// cloudRPCSpy tracks cloud RPC calls made during wizard operations
type cloudRPCSpy struct {
	proto.UnimplementedAgentServer
	removeCalls      []string
	upsertCalls      []string
	setActiveCalls   []string
	getProfilesCalls int
}

func (s *cloudRPCSpy) GetCloudProfiles(context.Context, *proto.GetCloudProfilesRequest) (*proto.GetCloudProfilesResponse, error) {
	s.getProfilesCalls++
	return &proto.GetCloudProfilesResponse{Profiles: []*proto.CloudProfileInfo{
		{Name: "existing-account"},
	}}, nil
}

func (s *cloudRPCSpy) RemoveCloudProfile(_ context.Context, req *proto.RemoveCloudProfileRequest) (*proto.RemoveCloudProfileResponse, error) {
	s.removeCalls = append(s.removeCalls, req.GetName())
	return &proto.RemoveCloudProfileResponse{Ok: true}, nil
}

func (s *cloudRPCSpy) SetActiveCloudProfile(_ context.Context, req *proto.SetActiveCloudProfileRequest) (*proto.SetActiveCloudProfileResponse, error) {
	s.setActiveCalls = append(s.setActiveCalls, req.GetName())
	return &proto.SetActiveCloudProfileResponse{Ok: true}, nil
}

func (s *cloudRPCSpy) UpsertCloudProfile(_ context.Context, req *proto.UpsertCloudProfileRequest) (*proto.UpsertCloudProfileResponse, error) {
	s.upsertCalls = append(s.upsertCalls, req.GetName())
	return &proto.UpsertCloudProfileResponse{Ok: true}, nil
}

func TestWizardAbandonDoesNotMakeCloudRPCs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	spy := &cloudRPCSpy{}
	proto.RegisterAgentServer(server, spy)
	go server.Serve(listener)
	defer server.Stop()
	defer listener.Close()

	client, err := agentclient.DialExisting(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	wp := &wizardPage{
		agent: client,
		state: wizard.State{
			LocusMode: "cloud_primary",
			Step:      wizard.StepCloud,
		},
	}

	// Simulate a mid-wizard state and abandon
	pressQ(t, wp) // First q arms
	if closed := pressQ(t, wp); !closed {
		t.Fatal("second q: want page closed")
	}

	// Verify no cloud RPCs were made during abandon
	if spy.getProfilesCalls > 0 {
		t.Errorf("abandon made %d GetCloudProfiles calls, expected 0", spy.getProfilesCalls)
	}
	if len(spy.removeCalls) > 0 {
		t.Errorf("abandon made %d RemoveCloudProfile calls: %v, expected 0", len(spy.removeCalls), spy.removeCalls)
	}
	if len(spy.upsertCalls) > 0 {
		t.Errorf("abandon made %d UpsertCloudProfile calls: %v, expected 0", len(spy.upsertCalls), spy.upsertCalls)
	}
	if len(spy.setActiveCalls) > 0 {
		t.Errorf("abandon made %d SetActiveCloudProfile calls: %v, expected 0", len(spy.setActiveCalls), spy.setActiveCalls)
	}
}

func TestWizardAbandonTwiceStillNoCloudRPCs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	spy := &cloudRPCSpy{}
	proto.RegisterAgentServer(server, spy)
	go server.Serve(listener)
	defer server.Stop()
	defer listener.Close()

	client, err := agentclient.DialExisting(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	wp := &wizardPage{
		agent: client,
		state: wizard.State{
			LocusMode: "cloud_primary",
			Step:      wizard.StepCloud,
		},
	}

	// First abandon
	pressQ(t, wp)
	if closed := pressQ(t, wp); !closed {
		t.Fatal("first abandon: want page closed")
	}

	// Second abandon (should close immediately since there's no state to clear)
	wp2 := &wizardPage{
		agent: client,
		state: wizard.State{
			LocusMode: "cloud_primary",
			Step:      wizard.StepCloud,
		},
	}
	pressQ(t, wp2)
	if closed := pressQ(t, wp2); !closed {
		t.Fatal("second abandon: want page closed (no state to persist)")
	}

	// Verify no cloud RPCs were made during either abandon
	if spy.getProfilesCalls > 0 {
		t.Errorf("two abandons made %d GetCloudProfiles calls, expected 0", spy.getProfilesCalls)
	}
	if len(spy.removeCalls) > 0 {
		t.Errorf("two abandons made %d RemoveCloudProfile calls: %v, expected 0", len(spy.removeCalls), spy.removeCalls)
	}
	if len(spy.upsertCalls) > 0 {
		t.Errorf("two abandons made %d UpsertCloudProfile calls: %v, expected 0", len(spy.upsertCalls), spy.upsertCalls)
	}
	if len(spy.setActiveCalls) > 0 {
		t.Errorf("two abandons made %d SetActiveCloudProfile calls: %v, expected 0", len(spy.setActiveCalls), spy.setActiveCalls)
	}
}

func TestWizardAbandonWithDifferentCurrentMetadata(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	spy := &cloudRPCSpy{
		// Simulate that current metadata differs from what wizard would have captured
	}
	proto.RegisterAgentServer(server, spy)
	go server.Serve(listener)
	defer server.Stop()
	defer listener.Close()

	client, err := agentclient.DialExisting(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	wp := &wizardPage{
		agent: client,
		state: wizard.State{
			LocusMode: "cloud_primary",
			Step:      wizard.StepCloud,
		},
	}

	// Abandon should not make any cloud RPCs even if current metadata differs
	pressQ(t, wp)
	if closed := pressQ(t, wp); !closed {
		t.Fatal("abandon: want page closed")
	}

	// Verify no cloud RPCs were made
	if spy.getProfilesCalls > 0 {
		t.Errorf("abandon with different metadata made %d GetCloudProfiles calls, expected 0", spy.getProfilesCalls)
	}
	if len(spy.removeCalls) > 0 {
		t.Errorf("abandon with different metadata made %d RemoveCloudProfile calls: %v, expected 0", len(spy.removeCalls), spy.removeCalls)
	}
	if len(spy.upsertCalls) > 0 {
		t.Errorf("abandon with different metadata made %d UpsertCloudProfile calls: %v, expected 0", len(spy.upsertCalls), spy.upsertCalls)
	}
	if len(spy.setActiveCalls) > 0 {
		t.Errorf("abandon with different metadata made %d SetActiveCloudProfile calls: %v, expected 0", len(spy.setActiveCalls), spy.setActiveCalls)
	}
}

func TestWizardAutofillPreservesExplicitChoice(t *testing.T) {
	wp := newTestWizardPage(t)
	wp.state.LocusMode = "open_only"
	wp.state.TierPicks = map[string]string{"llama_server.everyday": "custom-installed-model"}
	wp.autofillTiers()
	if got := wp.state.TierPicks["llama_server.everyday"]; got != "custom-installed-model" {
		t.Fatalf("explicit choice replaced: %q", got)
	}
}
func TestWizardAutofillCatalogWithoutShippedRecs(t *testing.T) {
	wp := newTestWizardPage(t)
	wp.state.LocusMode = "open_only"
	wp.recsOK = false
	wp.catalogOK = true
	wp.catalog.RecommendedOpenModels = map[string]string{"everyday": "catalog-choice"}
	wp.autofillTiers()
	if got := wp.state.TierPicks["llama_server.everyday"]; got != "catalog-choice" {
		t.Fatalf("catalog recommendation missing: %q", got)
	}
}

func TestWizardAutofillPreservesConfiguredLocalChoice(t *testing.T) {
	wp := newTestWizardPage(t)
	wp.state.LocusMode = "open_only"
	wp.curConfig = &agentclient.Config{ModelTiers: map[string]string{"llama_server.everyday": "configured-model"}}
	wp.autofillTiers()
	if got := wp.state.TierPicks["llama_server.everyday"]; got != "configured-model" {
		t.Fatalf("configured choice replaced: %q", got)
	}
}

// cloudRPCSpy uses only in-memory RPC fixtures, never host credentials.
func (*cloudRPCSpy) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
