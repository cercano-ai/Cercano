package server

import (
	"bytes"
	"context"
	"fmt"

	cfgsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/metadata"
	pb "google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
)

// Settings are shared, but the caller still belongs to an exact retained owner.
// Count the entire mutation (including waiting for configApplyMu) against that
// owner. This never acquires an owner or queues a transfer on busy refusal.
func (s *Server) beginSettingsWork(ctx context.Context) (context.Context, *foregroundWork, error) {
	if s.coexistence == nil {
		return ctx, nil, nil
	}
	if s.cfgSvc.Frozen() {
		return nil, nil, cfgsvc.ErrFrozen
	}
	md, _ := metadata.FromIncomingContext(ctx)
	ids := md.Get("cercano-conversation-id")
	if len(ids) != 1 {
		return nil, nil, fmt.Errorf("exact terminal binding required for settings")
	}
	return s.beginForeground(ctx, ids[0])
}

// Keep provider/runtime, credential and producer activation behind their existing
// integration gates. Unknown protobuf fields are refused too (fail closed when
// a newer client introduces a setting). No rejected file revision is adopted:
// otherwise a later save could silently overwrite an unapplied external edit.
func (s *Server) validateSharedSettings(req *proto.UpdateConfigRequest, authoritative *config.Config) error {
	if s.cfgSvc.Frozen() {
		return cfgsvc.ErrFrozen
	}
	rest := pb.Clone(req).(*proto.UpdateConfigRequest)
	rest.ToolLoopMaxIterations = ""
	rest.ElideToolResults = ""
	rest.LossyToolElision = ""
	if !pb.Equal(rest, &proto.UpdateConfigRequest{}) {
		return fmt.Errorf("shared settings: only tool loop and tool-result elision are integrated; change not applied")
	}
	if authoritative == nil {
		return nil
	}
	before := s.cfgSvc.Get()
	after := authoritative.Clone()
	after.ToolLoop.MaxIterations = before.ToolLoop.MaxIterations
	after.Compaction.ElideToolResults = before.Compaction.ElideToolResults
	after.Compaction.LossyToolElision = before.Compaction.LossyToolElision
	// YAML compares persisted semantics, excluding opaque coordinator metadata.
	a, err := yaml.Marshal(before)
	if err != nil {
		return err
	}
	b, err := yaml.Marshal(after)
	if err != nil {
		return err
	}
	if !bytes.Equal(a, b) {
		return fmt.Errorf("shared settings: file contains changes requiring restart or further integration; published revision not applied")
	}
	return nil
}
