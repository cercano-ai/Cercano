package worker

import (
	"cercano/source/server/internal/credentialtest"
	"cercano/source/server/internal/modelevidence"
	"cercano/source/server/internal/modelmetadata"
	"cercano/source/server/internal/toolstack"
	"cercano/source/server/pkg/config"
	"testing"
)

func TestSubscriptionVisionSurvivesWorkerSnapshot(t *testing.T) {
	credentialtest.Enable(t)
	c := config.Defaults()
	c.LocusMode = "cloud_only"
	c.ActiveCloudProfile = "signed-in"
	c.CloudProfiles = []config.CloudProfile{{Name: "signed-in", Flavor: "responses", Route: "chatgpt", TierOverrides: map[config.CostTier]string{config.CostStandard: "gpt-6-astra"}}}
	p := c.CloudProfiles[0]
	var ids []modelmetadata.Identity
	for _, model := range c.ModelProfiles.CloudImageCandidates(p, config.TierEveryday) {
		ids = append(ids, modelevidence.IdentityFor(p, model))
	}
	snapshot := modelevidence.New(nil).Snapshot(t.Context(), ids)
	evidence := UnmarshalModelMetadata(MarshalModelMetadata(snapshot))
	restored := ConfigFromSnapshot(SnapshotConfig(c, "", nil))
	lookup := func(p config.CloudProfile, model string) modelmetadata.Evidence {
		ev, _ := evidence.Lookup(modelevidence.IdentityFor(p, model))
		return ev
	}
	r, err := buildWorkerProviders(t.Context(), restored, nil, nil, nil, lookup)
	if err != nil {
		t.Fatal(err)
	}
	target, ok := toolstack.ResolveCloudVision(r.Cloud())
	if !ok || target.Model != "gpt-6-astra" {
		t.Fatalf("automatic worker target: %+v available=%v", target, ok)
	}
	other := restored.CloudProfiles[0]
	other.Name = "another-account"
	if lookup(other, "gpt-6-astra").Vision != modelmetadata.VisionUnknown {
		t.Fatal("evidence leaked between accounts")
	}
	if restored.CloudProfiles[0].ImageModel != "" {
		t.Fatal("resolution pinned image model")
	}
}
