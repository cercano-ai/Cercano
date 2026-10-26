package ui

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"cercano/source/clients/cli/internal/theme"
	"cercano/source/clients/cli/internal/wizard"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

// This file exercises integrated setup scenarios against a recording fake
// agent over real loopback gRPC — the same path production uses — to pin the
// preservation and re-entry contracts end-to-end:
//
//   - a user with one supported cloud account accepts a complete three-tier
//     recommendation with no other account, no download, and no account write;
//   - a local-only run introduces no cloud routes, backups, or profile writes;
//   - an existing user rerunning the wizard keeps unrelated configuration,
//     explicit assignments, and all accounts;
//   - cancellation keeps connected accounts and clears only local resume
//     state; and
//   - no guided path — accept, cancel, or re-entry — ever invokes the
//     destructive setup-reset RPC.

// wizardScenarioStub is a recording agent: it owns a fake cloud-account
// store and sparse config, records every mutating RPC, and never stores or
// echoes API keys (SetCloudProfileKey records the profile name only — no
// credential material is ever captured by the fixture).
type wizardScenarioStub struct {
	proto.UnimplementedAgentServer
	mu     sync.Mutex
	client *agentclient.Client

	// Server-side fake state.
	locusMode      string
	modelTiers     map[string]string
	profiles       []*proto.CloudProfileInfo
	active         string
	routing        *proto.RoutingAssignments
	routingUpdates []*proto.UpdateRoutingAssignmentsRequest
	catalog        *proto.ListRuntimeModelsResponse

	// Recorded mutations.
	updates   []*proto.UpdateConfigRequest
	upserts   []*proto.UpsertCloudProfileRequest
	keyNames  []string // profile names for SetCloudProfileKey; never the key
	activated []string
	removals  []string
	downloads []*proto.DownloadRuntimeModelRequest
	resets    int
}

func newScenarioStub() *wizardScenarioStub {
	return &wizardScenarioStub{
		modelTiers: map[string]string{},
		profiles: []*proto.CloudProfileInfo{
			{
				Name:     "personal",
				Provider: "anthropic",
				HasKey:   true,
				Route:    "direct",
				Backend:  "chat_completions",
				ModelChoices: &proto.ProfileModelChoices{
					TierOverrides: map[string]string{},
				},
				EffectiveQualityModels: map[string]string{
					"premium":  "claude-opus-4",
					"standard": "claude-sonnet-4",
					"economy":  "claude-haiku-4",
				},
			},
			{
				Name:     "team-external",
				Provider: "anthropic",
				HasKey:   true,
				ModelChoices: &proto.ProfileModelChoices{
					TierOverrides: map[string]string{},
				},
				EffectiveQualityModels: map[string]string{
					"premium":  "team-opus",
					"standard": "team-sonnet",
					"economy":  "team-haiku",
				},
			},
		},
		active: "personal",
	}
}

func (s *wizardScenarioStub) GetConfig(context.Context, *proto.GetConfigRequest) (*proto.GetConfigResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tiers := map[string]string{}
	for k, v := range s.modelTiers {
		tiers[k] = v
	}
	return &proto.GetConfigResponse{LocusMode: s.locusMode, ModelTiers: tiers}, nil
}

func (s *wizardScenarioStub) UpdateConfig(_ context.Context, req *proto.UpdateConfigRequest) (*proto.UpdateConfigResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, req)
	if req.GetLocusMode() != "" {
		s.locusMode = req.GetLocusMode()
	}
	if k := req.GetModelTierKey(); k != "" {
		s.modelTiers[k] = req.GetModelTierValue()
	}
	return &proto.UpdateConfigResponse{Success: true}, nil
}

func (s *wizardScenarioStub) GetCloudProviders(context.Context, *proto.GetCloudProvidersRequest) (*proto.GetCloudProvidersResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &proto.GetCloudProvidersResponse{
		Providers: []*proto.CloudProvider{{
			Id: "anthropic", Label: "anthropic", Tier: "verified", Backend: "chat_completions", Route: "direct",
		}},
		Active: s.active, Assignments: s.routing, CustomProfiles: s.profiles,
	}, nil
}

func (s *wizardScenarioStub) UpdateRoutingAssignments(_ context.Context, req *proto.UpdateRoutingAssignmentsRequest) (*proto.UpdateRoutingAssignmentsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routingUpdates = append(s.routingUpdates, req)
	s.routing = req.Assignments
	s.active = req.Assignments.Primary
	return &proto.UpdateRoutingAssignmentsResponse{Ok: true}, nil
}

func (s *wizardScenarioStub) GetCloudProfiles(context.Context, *proto.GetCloudProfilesRequest) (*proto.GetCloudProfilesResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*proto.CloudProfileInfo, len(s.profiles))
	for i, p := range s.profiles {
		out[i] = cloneScenarioProfile(p)
	}
	return &proto.GetCloudProfilesResponse{Profiles: out, Active: s.active}, nil
}

func (s *wizardScenarioStub) UpsertCloudProfile(_ context.Context, req *proto.UpsertCloudProfileRequest) (*proto.UpsertCloudProfileResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.upserts = append(s.upserts, req)
	for _, p := range s.profiles {
		if p.Name == req.GetName() {
			if req.GetCreateOnly() {
				return &proto.UpsertCloudProfileResponse{Ok: false, Error: "profile already exists"}, nil
			}
			if mc := req.GetModelChoices(); mc != nil {
				if p.ModelChoices == nil {
					p.ModelChoices = &proto.ProfileModelChoices{TierOverrides: map[string]string{}}
				}
				for q, m := range mc.GetTierOverrides() {
					if m == "" {
						delete(p.ModelChoices.TierOverrides, q)
					} else {
						p.ModelChoices.TierOverrides[q] = m
					}
				}
				if mc.GetImageModel() != "" {
					p.ModelChoices.ImageModel = mc.GetImageModel()
				}
			}
			return &proto.UpsertCloudProfileResponse{Ok: true}, nil
		}
	}
	s.profiles = append(s.profiles, &proto.CloudProfileInfo{
		Name: req.GetName(), Provider: req.GetProvider(), Backend: req.GetBackend(),
		BaseUrl: req.GetBaseUrl(), Route: req.GetRoute(), HasKey: false,
		ModelChoices: &proto.ProfileModelChoices{TierOverrides: map[string]string{}},
	})
	return &proto.UpsertCloudProfileResponse{Ok: true}, nil
}

func (s *wizardScenarioStub) SetCloudProfileKey(_ context.Context, req *proto.SetCloudProfileKeyRequest) (*proto.SetCloudProfileKeyResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keyNames = append(s.keyNames, req.GetName()) // never the key material
	return &proto.SetCloudProfileKeyResponse{Ok: true}, nil
}

func (s *wizardScenarioStub) SetActiveCloudProfile(_ context.Context, req *proto.SetActiveCloudProfileRequest) (*proto.SetActiveCloudProfileResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activated = append(s.activated, req.GetName())
	s.active = req.GetName()
	return &proto.SetActiveCloudProfileResponse{Ok: true}, nil
}

func (s *wizardScenarioStub) RemoveCloudProfile(_ context.Context, req *proto.RemoveCloudProfileRequest) (*proto.RemoveCloudProfileResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removals = append(s.removals, req.GetName())
	kept := s.profiles[:0]
	for _, p := range s.profiles {
		if p.Name != req.GetName() {
			kept = append(kept, p)
		}
	}
	s.profiles = kept
	return &proto.RemoveCloudProfileResponse{Ok: true}, nil
}

func (s *wizardScenarioStub) ResetSetup(context.Context, *proto.ResetSetupRequest) (*proto.ResetSetupResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resets++
	return &proto.ResetSetupResponse{Ok: true}, nil
}

func (s *wizardScenarioStub) DownloadRuntimeModel(_ context.Context, req *proto.DownloadRuntimeModelRequest) (*proto.DownloadRuntimeModelResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downloads = append(s.downloads, req)
	return &proto.DownloadRuntimeModelResponse{Ok: true}, nil
}

func (s *wizardScenarioStub) GetOpenRuntimeStatus(context.Context, *proto.GetOpenRuntimeStatusRequest) (*proto.GetOpenRuntimeStatusResponse, error) {
	return &proto.GetOpenRuntimeStatusResponse{Status: &proto.OpenRuntimeStatus{Ok: true, Runtime: "llama_server"}}, nil
}

// ListRuntimeModels serves a 128 GiB unified-memory catalog: model A
// (recommended for the three capability tiers) is not on disk yet; the small
// fast-tier and embedding models already are.
func (s *wizardScenarioStub) ListRuntimeModels(context.Context, *proto.ListRuntimeModelsRequest) (*proto.ListRuntimeModelsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.catalog != nil {
		return s.catalog, nil
	}
	return &proto.ListRuntimeModelsResponse{
		SystemRamBytes:     137438953472,
		MemoryTopology:     "unified",
		UnifiedMemoryBytes: 137438953472,
		HardwareOk:         true,
		RecommendedOpenModels: map[string]string{
			"most_capable":    "llama_server:catalog:glm-4.5-air-q4_k_m",
			"everyday":        "llama_server:catalog:glm-4.5-air-q4_k_m",
			"fast_light":      "llama_server:catalog:gemma-3-4b-it-q4_k_m",
			"fast_light_text": "llama_server:catalog:gemma-3-4b-it-q4_k_m",
			"embedding":       "llama_server:catalog:nomic-embed-text-v1.5-f16",
		},
		Models: []*proto.RuntimeModel{
			{Id: "llama_server:catalog:glm-4.5-air-q4_k_m", DisplayName: "GLM-4.5-Air Q4_K_M", Runtime: "llama_server", Source: "catalog", SizeBytes: 72975748384, DownloadState: "", SupportsChat: true},
			{Id: "llama_server:catalog:gemma-3-4b-it-q4_k_m", DisplayName: "Gemma 3 4B Q4_K_M", Runtime: "llama_server", Source: "catalog", SizeBytes: 3000000000, DownloadState: "downloaded", SupportsChat: true},
			{Id: "llama_server:catalog:nomic-embed-text-v1.5-f16", DisplayName: "nomic-embed-text v1.5 F16", Runtime: "llama_server", Source: "catalog", SizeBytes: 274290560, DownloadState: "downloaded", SupportsEmbed: true},
		},
	}, nil
}

func cloneScenarioProfile(p *proto.CloudProfileInfo) *proto.CloudProfileInfo {
	out := &proto.CloudProfileInfo{
		Name: p.GetName(), Provider: p.GetProvider(), HasKey: p.GetHasKey(),
		Route: p.GetRoute(), Backend: p.GetBackend(), BaseUrl: p.GetBaseUrl(),
	}
	eqm := map[string]string{}
	for k, v := range p.GetEffectiveQualityModels() {
		eqm[k] = v
	}
	out.EffectiveQualityModels = eqm
	if mc := p.GetModelChoices(); mc != nil {
		ov := map[string]string{}
		for k, v := range mc.GetTierOverrides() {
			ov[k] = v
		}
		out.ModelChoices = &proto.ProfileModelChoices{TierOverrides: ov, ImageModel: mc.GetImageModel()}
	}
	return out
}

// newScenario isolates HOME and the wizard resume file, spins up the
// recording fake agent over loopback gRPC, and dials a real agentclient at
// it. Tests mutate the stub's server-side state and then construct the
// wizard page so its read-only discovery sees the mutation.
func newScenario(t *testing.T) (*wizardScenarioStub, *agentclient.Client) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("CERCANO_WIZARD_STATE", filepath.Join(dir, "wizard_state.yaml"))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	stub := newScenarioStub()
	proto.RegisterAgentServer(server, stub)
	go server.Serve(listener)
	t.Cleanup(func() { server.Stop(); listener.Close() })
	client, err := agentclient.DialExisting(context.Background(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	stub.client = client
	return stub, client
}

// scenarioWizard retains the legacy scoped-apply fixture. New resource flow
// tests instantiate the production constructor without this compatibility shim.
func scenarioWizard(t *testing.T, client *agentclient.Client) *wizardPage {
	t.Helper()
	p := theme.BuiltinThemes()[0]
	saved, resumed := wizard.Load()
	wp := newWizardPage(client, p.Palette, theme.NewStyles(p.Palette), 100, 40)
	if resumed {
		wp.state = saved
	} else {
		wp.state = wizard.State{Step: wizard.StepLocus}
	}
	wp.state.FlowVersion = 0
	if wp.state.Step == wizard.StepOpen {
		wp.autofillTiers()
	}
	wp.cursor = wp.defaultCursor()
	return wp
}

// pressRows moves the cursor onto the row with the given key and presses
// enter, failing loudly if the row is absent (so step-layout changes surface
// here instead of silently passing).
func pressRows(t *testing.T, wp *wizardPage, key string) bool {
	t.Helper()
	rows := wp.rows()
	at := -1
	for i, r := range rows {
		if r.Key == key && !r.Disabled {
			at = i
			break
		}
	}
	if at < 0 {
		t.Fatalf("no enabled row %q on step %s", key, wp.state.Step)
	}
	// Move toward the target from wherever the cursor sits — rows above the
	// cursor (e.g. overriding the recommended locus) need KeyUp, not KeyDown.
	guard := 0
	for wp.cursor != at {
		code := tea.KeyDown
		if at < wp.cursor {
			code = tea.KeyUp
		}
		wp.Update(tea.KeyPressMsg{Code: code})
		guard++
		if guard > 2*len(rows)+2 {
			t.Fatalf("cursor never reached row %q (at %d, want %d)", key, wp.cursor, at)
		}
	}
	return press(t, wp, tea.KeyEnter)
}

// assertNeverReset is the destructive-reset guard every scenario shares:
// guided setup (including reruns) must never invoke ResetSetup.
func (s *wizardScenarioStub) assertNeverReset(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.resets != 0 {
		t.Fatalf("guided setup invoked destructive setup reset %d time(s); reruns must be nondestructive", s.resets)
	}
}

// assertNoAccountWrites pins that connected accounts were left untouched:
// no profile create/update, activation, key change, or removal.
func (s *wizardScenarioStub) assertNoAccountWrites(t *testing.T) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := len(s.upserts); n > 0 {
		t.Fatalf("unexpected profile writes: %d upsert(s): %v", n, s.upserts[0].GetName())
	}
	if n := len(s.keyNames); n > 0 {
		t.Fatalf("unexpected credential writes for %v", s.keyNames)
	}
	if n := len(s.activated); n > 0 {
		t.Fatalf("unexpected activations: %v", s.activated)
	}
	if n := len(s.removals); n > 0 {
		t.Fatalf("account(s) removed by the wizard: %v", s.removals)
	}
}

func (s *wizardScenarioStub) assertProfilesIntact(t *testing.T, want ...string) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	got := map[string]bool{}
	for _, p := range s.profiles {
		got[p.GetName()] = true
		if p.GetName() == "personal" && !p.GetHasKey() {
			t.Fatal("active account lost its credential marker")
		}
	}
	for _, name := range want {
		if !got[name] {
			t.Fatalf("account %q missing after wizard run (have %v)", name, got)
		}
	}
}

func (s *wizardScenarioStub) writeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.routingUpdates) + len(s.updates) + len(s.upserts) + len(s.keyNames) + len(s.activated) + len(s.removals) + len(s.downloads)
}

// TestWizardScenarioCloudOnlyOneAccountFullAccept: a user with one working
// cloud account accepts the complete three-tier recommendation. No other
// account is required, nothing downloads, and no account or unrelated
// configuration is written.
func TestWizardScenarioCloudOnlyOneAccountFullAccept(t *testing.T) {
	stub, client := newScenario(t)
	stub.mu.Lock()
	stub.profiles = stub.profiles[:1] // This scenario really has only one account.
	stub.mu.Unlock()
	wp := scenarioWizard(t, client)
	if wp.state.Step != wizard.StepLocus {
		t.Fatalf("fresh run should start at locus, got %s", wp.state.Step)
	}
	if got := wp.recommendedLocus(); got != "cloud_only" {
		t.Fatalf("resource discovery should recommend cloud_only for an account-holding user, got %q", got)
	}
	if closed := pressRows(t, wp, wp.recommendedLocus()); closed {
		t.Fatal("recommendation must stop at preview")
	}
	if stub.writeCount() != 0 {
		t.Fatal("recommendation wrote before confirmation")
	}
	if wp.state.Step != wizard.StepDone {
		t.Fatalf("cloud_only: want done step, got %s", wp.state.Step)
	}
	// The preview shows the complete three-tier recommendation from the one
	// account, explicitly not yet applied.
	summary := wp.summary()
	for _, label := range []string{"Deep reasoning", "Everyday development", "Lightweight tasks"} {
		if !strings.Contains(summary, label) {
			t.Fatalf("preview missing %q tier:\n%s", label, summary)
		}
	}
	if !strings.Contains(summary, "not yet applied") {
		t.Fatal("preview does not mark itself as not yet applied")
	}
	if strings.Contains(summary, "Download") {
		t.Fatalf("cloud-only preview should promise no downloads:\n%s", summary)
	}
	if closed := press(t, wp, tea.KeyEnter); !closed {
		t.Fatal("apply should close the wizard")
	}
	stub.mu.Lock()
	updates := append([]*proto.UpdateConfigRequest(nil), stub.updates...)
	downloads := len(stub.downloads)
	stub.mu.Unlock()
	if len(updates) != 1 || updates[0].GetLocusMode() != "cloud_only" {
		var keys []string
		for _, u := range updates {
			keys = append(keys, u.GetModelTierKey())
		}
		t.Fatalf("cloud-only accept should write exactly the locus patch; got %d updates, tier keys %v", len(updates), keys)
	}
	if downloads != 0 {
		t.Fatalf("cloud-only accept enrolled %d download(s)", downloads)
	}
	stub.assertNoAccountWrites(t)
	stub.assertProfilesIntact(t, "personal")
	stub.assertNeverReset(t)
	if _, ok := wizard.Load(); ok {
		t.Fatal("accept should clear the resume file")
	}
}

// TestWizardScenarioLocalOnlyNoCloudWritesOrResets: an open_only rerun
// resumes mid-run, autofills from the curated catalog, downloads only the
// missing distinct artifact, and writes no cloud slot, no cloud profile
// change, and no destructive reset. Server-side unrelated configuration
// survives.
func TestWizardScenarioLocalOnlyNoCloudWritesOrResets(t *testing.T) {
	stub, client := newScenario(t)
	stub.mu.Lock()
	stub.locusMode = "open_only"
	stub.modelTiers["unrelated.key"] = "keep-me"
	stub.mu.Unlock()
	if err := wizard.Save(wizard.State{
		Step:      wizard.StepOpen,
		LocusMode: "open_only",
		TierPicks: map[string]string{"llama_server.everyday": "GLM-4.5-Air Q4_K_M"},
	}); err != nil {
		t.Fatal(err)
	}
	wp := scenarioWizard(t, client) // re-entry: resumes the saved mid-run state
	if wp.state.Step != wizard.StepOpen {
		t.Fatalf("resume should land on the open step, got %s", wp.state.Step)
	}
	// Autofill filled the remaining slots from the curated catalog.
	for _, key := range []string{"llama_server.most_capable", "llama_server.fast_light", "llama_server.fast_light_text", "llama_server.embedding"} {
		if wp.state.TierPicks[key] == "" {
			t.Fatalf("autofill left %s empty", key)
		}
	}
	if closed := pressRows(t, wp, "continue"); closed {
		t.Fatal("continue closed the page")
	}
	summary := wp.summary()
	if !strings.Contains(summary, "no cloud routes or backups") {
		t.Fatalf("local-only preview must disclose no cloud routes or backups:\n%s", summary)
	}
	if !strings.Contains(summary, "Download after confirmation: llama_server:catalog:glm-4.5-air-q4_k_m") {
		t.Fatalf("preview must disclose the missing download before apply:\n%s", summary)
	}
	if closed := press(t, wp, tea.KeyEnter); !closed {
		t.Fatal("apply should close the wizard")
	}
	stub.mu.Lock()
	if len(stub.updates) == 0 {
		stub.mu.Unlock()
		t.Fatal("apply wrote nothing")
	}
	if stub.updates[0].GetLocusMode() != "open_only" || stub.updates[0].GetOpenRuntime() != "llama_server" {
		stub.mu.Unlock()
		t.Fatalf("finish patch wrong: locus=%q runtime=%q", stub.updates[0].GetLocusMode(), stub.updates[0].GetOpenRuntime())
	}
	for _, u := range stub.updates[1:] {
		if k := u.GetModelTierKey(); !strings.HasPrefix(k, "llama_server.") {
			stub.mu.Unlock()
			t.Fatalf("local-only apply wrote non-local slot %q", k)
		}
	}
	if stub.modelTiers["unrelated.key"] != "keep-me" {
		stub.mu.Unlock()
		t.Fatal("local-only apply clobbered unrelated server-side configuration")
	}
	downloads := len(stub.downloads)
	stub.mu.Unlock()
	if downloads != 1 {
		t.Fatalf("want exactly one distinct missing download (deduped), got %d", downloads)
	}
	stub.assertNoAccountWrites(t)
	stub.assertProfilesIntact(t, "personal", "team-external")
	stub.assertNeverReset(t)
}

// TestWizardScenarioExistingUserRerunPreservesUnrelatedConfig: an existing
// cloud_only user reruns the wizard, switches to the mixed mode cloud_primary,
// and accepts. The rerun reuses the existing account without authentication,
// keeps the explicit local assignment it discovers, never touches unrelated
// config keys server-side, and never resets. (open_primary skips the cloud
// step — see wizard's TestOpenPathsSkipCloudStep — so the rerun-with-reuse
// scenario walks cloud_primary instead.)
func TestWizardScenarioExistingUserRerunPreservesUnrelatedConfig(t *testing.T) {
	stub, client := newScenario(t)
	stub.mu.Lock()
	stub.locusMode = "cloud_only"
	stub.modelTiers["unrelated.key"] = "keep-me"
	stub.modelTiers["llama_server.everyday"] = "my-custom-pick"
	stub.mu.Unlock()
	wp := scenarioWizard(t, client) // rerun: fresh page, existing user's config
	if got := wp.recommendedLocus(); got != "cloud_only" {
		t.Fatalf("rerun should recommend the standing mode cloud_only, got %q", got)
	}
	if closed := pressRows(t, wp, "cloud_primary"); closed {
		t.Fatal("locus selection closed the page")
	}
	if wp.state.Step != wizard.StepDone || wp.state.AuthMethod != "existing" {
		t.Fatalf("template should reuse account and preview: %+v", wp.state)
	}
	pressRows(t, wp, "edit_models")
	if wp.state.Step != wizard.StepOpen {
		t.Fatal("model customization did not open")
	}

	if got := wp.state.TierPicks["llama_server.everyday"]; got != "my-custom-pick" {
		t.Fatalf("rerun autofill lost the explicit assignment: %q", got)
	}
	if closed := pressRows(t, wp, "continue"); closed {
		t.Fatal("continue closed the page")
	}
	summary := wp.summary()
	if !strings.Contains(summary, "Work may leave this machine") {
		t.Fatalf("mixed preview must disclose cloud use:\n%s", summary)
	}
	if !strings.Contains(summary, "Assignments remain editable in /config") {
		t.Fatalf("preview must point at existing configuration surfaces:\n%s", summary)
	}
	if closed := press(t, wp, tea.KeyEnter); !closed {
		t.Fatal("apply should close the wizard")
	}
	stub.mu.Lock()
	for _, u := range stub.updates[1:] {
		if u.GetModelTierKey() == "unrelated.key" {
			stub.mu.Unlock()
			t.Fatal("apply touched a config key the wizard does not own")
		}
	}
	if stub.modelTiers["unrelated.key"] != "keep-me" {
		stub.mu.Unlock()
		t.Fatal("rerun clobbered unrelated server-side configuration")
	}
	if stub.modelTiers["llama_server.everyday"] != "my-custom-pick" {
		stub.mu.Unlock()
		t.Fatalf("explicit assignment not preserved server-side: %q", stub.modelTiers["llama_server.everyday"])
	}
	if stub.locusMode != "cloud_primary" {
		stub.mu.Unlock()
		t.Fatalf("locus not applied: %q", stub.locusMode)
	}
	stub.mu.Unlock()
	stub.assertNoAccountWrites(t)
	stub.assertProfilesIntact(t, "personal", "team-external")
	stub.assertNeverReset(t)
}

// TestWizardScenarioCancelPersistsAccountsClearsOnlyResume: a mid-run
// double-q cancellation keeps both accounts and the server-side
// configuration untouched, clears only the local resume state, and never
// resets.
func TestWizardScenarioCancelPersistsAccountsClearsOnlyResume(t *testing.T) {
	stub, client := newScenario(t)
	stub.mu.Lock()
	stub.locusMode = "cloud_primary"
	stub.mu.Unlock()
	wp := scenarioWizard(t, client)
	if closed := pressRows(t, wp, "cloud_primary"); closed {
		t.Fatal("locus selection closed the page")
	}
	pressRows(t, wp, "change_account")
	// The account-persistence contract is disclosed on the cloud step, where
	// account decisions happen — verify it before leaving that step.
	if !strings.Contains(wp.stepDesc(), "Connected accounts remain if you cancel setup") {
		t.Fatalf("cloud step must disclose the account-persistence contract:\n%s", wp.stepDesc())
	}
	if closed := pressRows(t, wp, "use_existing"); closed {
		t.Fatal("existing-account reuse closed the page")
	}
	pressRows(t, wp, "edit_models")
	if wp.state.Step != wizard.StepOpen {
		t.Fatalf("want open step, got %s", wp.state.Step)
	}
	if closed := press(t, wp, 'q'); closed {
		t.Fatal("first q should arm, not close")
	}
	if !wp.abandonArmed {
		t.Fatal("cancellation not armed")
	}
	if closed := press(t, wp, 'q'); !closed {
		t.Fatal("second q should abandon")
	}
	if n := stub.writeCount(); n != 0 {
		t.Fatalf("cancellation performed %d server write(s)", n)
	}
	stub.mu.Lock()
	locus := stub.locusMode
	stub.mu.Unlock()
	if locus != "cloud_primary" {
		t.Fatal("cancellation changed server-side configuration")
	}
	stub.assertProfilesIntact(t, "personal", "team-external")
	stub.assertNeverReset(t)
	if _, ok := wizard.Load(); ok {
		t.Fatal("cancellation must clear the local resume state")
	}
}

// TestWizardScenarioInterruptedResumeThenAbandon: esc on the first screen
// persists the run; re-entry resumes it; abandoning then clears only the
// resume file with zero server writes.
func TestWizardScenarioInterruptedResumeThenAbandon(t *testing.T) {
	stub, client := newScenario(t)
	wp := scenarioWizard(t, client)
	if closed := press(t, wp, tea.KeyEsc); !closed {
		t.Fatal("esc on the first screen should close the page")
	}
	st, ok := wizard.Load()
	if !ok || st.Step != wizard.StepLocus {
		t.Fatal("interrupted run did not persist for re-entry")
	}
	resumed := scenarioWizard(t, client)
	if resumed.state.Step != wizard.StepLocus {
		t.Fatalf("re-entry did not resume: %s", resumed.state.Step)
	}
	if closed := press(t, resumed, 'q'); closed {
		t.Fatal("first q should arm")
	}
	if closed := press(t, resumed, 'q'); !closed {
		t.Fatal("second q should abandon")
	}
	if n := stub.writeCount(); n != 0 {
		t.Fatalf("abandon performed %d server write(s)", n)
	}
	stub.assertNeverReset(t)
	if _, ok := wizard.Load(); ok {
		t.Fatal("abandon must clear the resume file")
	}
}

// TestWizardScenarioGuidedRerunNeverInvokesSetupReset: across accepts in all
// four locus modes followed by a rerun and a cancellation, the guided path
// never calls the destructive ResetSetup RPC — re-entry and reset stay
// separate surfaces.
func TestWizardScenarioGuidedRerunNeverInvokesSetupReset(t *testing.T) {
	for _, mode := range []string{"cloud_only", "cloud_primary", "open_primary", "open_only"} {
		t.Run(mode, func(t *testing.T) {
			stub, client := newScenario(t)
			wp := scenarioWizard(t, client)
			wp.state.LocusMode = mode
			wp.state.CloudProvider = "anthropic"
			wp.state.AuthMethod = "existing"
			for wp.state.Step != wizard.StepDone {
				if err := wp.state.Advance(); err != nil {
					t.Fatal(err)
				}
			}
			if err := wp.applyConfig(); err != nil {
				t.Fatal(err)
			}
			if err := wizard.Clear(); err != nil {
				t.Fatal(err)
			}
			// Re-enter (rerun), walk partway, then cancel.
			wp2 := scenarioWizard(t, client)
			wp2.state.LocusMode = mode
			wp2.state.CloudProvider = "anthropic"
			wp2.state.AuthMethod = "existing"
			for wp2.state.Step != wizard.StepDone {
				if err := wp2.state.Advance(); err != nil {
					t.Fatal(err)
				}
			}
			if closed := press(t, wp2, 'q'); closed {
				t.Fatal("first q should arm")
			}
			if closed := press(t, wp2, 'q'); !closed {
				t.Fatal("second q should abandon")
			}
			stub.assertNeverReset(t)
			stub.assertNoAccountWrites(t)
			stub.assertProfilesIntact(t, "personal", "team-external")
		})
	}
}

// Exercise the shortcut through real client RPCs, not only the customization
// path, with multiple accounts present and every existing placement policy.
func TestWizardScenarioRecommendedRerunAllModes(t *testing.T) {
	for _, mode := range []string{"cloud_only", "cloud_primary", "open_primary", "open_only"} {
		t.Run(mode, func(t *testing.T) {
			stub, client := newScenario(t)
			stub.mu.Lock()
			stub.locusMode = mode
			stub.modelTiers["llama_server.everyday"] = "preserved-explicit"
			stub.modelTiers["unrelated.key"] = "untouched"
			stub.mu.Unlock()
			wp := scenarioWizard(t, client)
			if pressRows(t, wp, wp.recommendedLocus()) {
				t.Fatal("closed before confirmation")
			}
			if wp.state.Step != wizard.StepDone {
				t.Fatalf("expected preview, got %s: %s", wp.state.Step, wp.status)
			}
			if n := stub.writeCount(); n != 0 {
				t.Fatalf("%d writes before confirmation", n)
			}
			if mode == "open_only" && !strings.Contains(wp.summary(), "no cloud routes or backups") {
				t.Fatal("missing local-only disclosure")
			}
			if wizard.ModeUsesOpen(mode) && !strings.Contains(wp.summary(), "preserved-explicit") {
				t.Fatal("preview lost explicit choice")
			}
			if !pressRows(t, wp, "finish") {
				t.Fatalf("confirmation failed: %s", wp.status)
			}
			stub.mu.Lock()
			gotMode := stub.locusMode
			unrelated := stub.modelTiers["unrelated.key"]
			explicit := stub.modelTiers["llama_server.everyday"]
			stub.mu.Unlock()
			if gotMode != mode || unrelated != "untouched" || explicit != "preserved-explicit" {
				t.Fatalf("preservation failed: %s, %s, %s", gotMode, unrelated, explicit)
			}
			stub.assertNoAccountWrites(t)
			stub.assertProfilesIntact(t, "personal", "team-external")
			stub.assertNeverReset(t)
		})
	}
}

// wizardScenarioStub uses only in-memory RPC fixtures, never host credentials.
func (*wizardScenarioStub) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: true}, nil
}
