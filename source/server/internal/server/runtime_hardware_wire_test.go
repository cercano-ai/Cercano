package server

import (
	"context"
	"testing"

	"cercano/source/server/internal/localruntime"
	"cercano/source/server/internal/localruntime/catalogdefaults"
	"cercano/source/server/internal/sysram"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
)

// These tests pin the probe-to-wire mapping of ListRuntimeModels using
// fake probe results — no live hardware is required, and no test here
// claims any specific platform or model compatibility.

// TestApplyHardwareProbesUnifiedWireResponse verifies that a unified
// probe (Apple Silicon style) is reported honestly on the wire: one
// pool, no fabricated GPU entries, and hardware_ok only when the RAM
// probe also succeeded.
func TestApplyHardwareProbesUnifiedWireResponse(t *testing.T) {
	resp := &proto.ListRuntimeModelsResponse{}
	applyHardwareProbes(resp, 64<<30, sysram.MemoryTopology{
		Kind:               sysram.TopologyUnified,
		UnifiedMemoryBytes: 64 << 30,
	})
	if resp.SystemRamBytes != 64<<30 {
		t.Errorf("unified: SystemRamBytes = %d, want %d", resp.SystemRamBytes, 64<<30)
	}
	if resp.MemoryTopology != sysram.TopologyUnified {
		t.Errorf("unified: MemoryTopology = %q, want %q", resp.MemoryTopology, sysram.TopologyUnified)
	}
	if resp.UnifiedMemoryBytes != 64<<30 {
		t.Errorf("unified: UnifiedMemoryBytes = %d, want %d", resp.UnifiedMemoryBytes, 64<<30)
	}
	if len(resp.Gpus) != 0 {
		t.Errorf("unified: fabricated GPU entries %v", resp.Gpus)
	}
	if !resp.HardwareOk {
		t.Errorf("unified: HardwareOk = false, want true when both probes succeeded")
	}
}

// TestApplyHardwareProbesDiscreteWireResponse verifies a discrete-GPU
// probe maps per-device — each GPU keeps its own pool, pools are never
// summed, and system RAM is never conflated with GPU memory.
func TestApplyHardwareProbesDiscreteWireResponse(t *testing.T) {
	resp := &proto.ListRuntimeModelsResponse{}
	applyHardwareProbes(resp, 128<<30, sysram.MemoryTopology{
		Kind: sysram.TopologyDiscrete,
		GPUs: []sysram.GPUDevice{
			{Name: "RTX 4090", MemoryBytes: 24 << 30},
			{Name: "T400", MemoryBytes: 0}, // size unknown, explicitly not zero memory
		},
	})
	if resp.MemoryTopology != sysram.TopologyDiscrete {
		t.Errorf("discrete: MemoryTopology = %q, want %q", resp.MemoryTopology, sysram.TopologyDiscrete)
	}
	if len(resp.Gpus) != 2 {
		t.Fatalf("discrete: got %d GPU entries, want 2", len(resp.Gpus))
	}
	if resp.Gpus[0].Name != "RTX 4090" || resp.Gpus[0].GpuMemoryBytes != 24<<30 {
		t.Errorf("discrete: first GPU = %v, want name RTX 4090 with 24 GiB", resp.Gpus[0])
	}
	if resp.Gpus[1].Name != "T400" || resp.Gpus[1].GpuMemoryBytes != 0 {
		t.Errorf("discrete: second GPU = %v, want name T400 with explicit unknown size 0", resp.Gpus[1])
	}
	if resp.UnifiedMemoryBytes != 0 {
		t.Errorf("discrete: UnifiedMemoryBytes = %d, want 0 (no unified pool)", resp.UnifiedMemoryBytes)
	}
	if !resp.HardwareOk {
		t.Errorf("discrete: HardwareOk = false, want true when RAM probe also succeeded")
	}
}

// TestApplyHardwareProbesUnknownWireResponse verifies a failed topology
// probe is disclosed on the wire — never silently upgraded to unified
// or discrete — and that hardware_ok stays false so clients do not
// present RAM-band recommendations as supported.
func TestApplyHardwareProbesUnknownWireResponse(t *testing.T) {
	resp := &proto.ListRuntimeModelsResponse{}
	applyHardwareProbes(resp, 32<<30, sysram.MemoryTopology{Kind: sysram.TopologyUnknown})
	if resp.MemoryTopology != sysram.TopologyUnknown {
		t.Errorf("unknown: MemoryTopology = %q, want %q", resp.MemoryTopology, sysram.TopologyUnknown)
	}
	if resp.UnifiedMemoryBytes != 0 {
		t.Errorf("unknown: UnifiedMemoryBytes = %d, want 0", resp.UnifiedMemoryBytes)
	}
	if len(resp.Gpus) != 0 {
		t.Errorf("unknown: unexpected GPU entries %v", resp.Gpus)
	}
	if resp.HardwareOk {
		t.Errorf("unknown: HardwareOk = true, want false when topology probe failed")
	}
}

// TestApplyHardwareProbesRAMProbeFailedWireResponse verifies a failed
// RAM probe keeps hardware_ok false even when the topology probe
// claims unified — the disclosure needs both probes.
func TestApplyHardwareProbesRAMProbeFailedWireResponse(t *testing.T) {
	resp := &proto.ListRuntimeModelsResponse{}
	applyHardwareProbes(resp, 0, sysram.MemoryTopology{
		Kind:               sysram.TopologyUnified,
		UnifiedMemoryBytes: 0,
	})
	if resp.HardwareOk {
		t.Errorf("ram-failed: HardwareOk = true, want false when RAM probe returned 0")
	}
}

// TestListRuntimeModelsHandlerWiresProbeConsistently exercises the real
// ListRuntimeModels handler end to end with a runtime manager attached
// (no live hardware claims: the machine's own probe results are used,
// and only internal consistency is asserted — the topology verdict, the
// RAM number, and the recommendation band must all agree).
func TestListRuntimeModelsHandlerWiresProbeConsistently(t *testing.T) {
	for _, runtime := range []string{"", "ollama", "llama_server", "mistralrs"} {
		t.Run("runtime="+runtime, func(t *testing.T) {
			srv := NewServer(nil, nil, nil, nil, nil)
			c := config.Config{OpenRuntime: runtime}
			srv.cfgSvc.Set(c)
			srv.SetRuntimeManager(localruntime.NewManager())
			resp, err := srv.ListRuntimeModels(context.Background(), &proto.ListRuntimeModelsRequest{})
			if err != nil {
				t.Fatalf("ListRuntimeModels: %v", err)
			}
			wantHardwareOk := resp.SystemRamBytes > 0 && resp.MemoryTopology != sysram.TopologyUnknown
			if resp.HardwareOk != wantHardwareOk {
				t.Errorf("HardwareOk = %v, want %v (ram=%d, topology=%q)",
					resp.HardwareOk, wantHardwareOk, resp.SystemRamBytes, resp.MemoryTopology)
			}
			switch resp.MemoryTopology {
			case sysram.TopologyUnified:
				// Unified reuses the RAM probe: one pool, one number.
				if resp.UnifiedMemoryBytes != resp.SystemRamBytes {
					t.Errorf("unified: UnifiedMemoryBytes %d != SystemRamBytes %d",
						resp.UnifiedMemoryBytes, resp.SystemRamBytes)
				}
			case sysram.TopologyDiscrete:
				if len(resp.Gpus) == 0 {
					t.Errorf("discrete: no GPU entries reported")
				}
			case sysram.TopologyUnknown:
				// Nothing claimed — that is the honest result.
			default:
				t.Errorf("unexpected MemoryTopology %q", resp.MemoryTopology)
			}
			// The RAM-band recommendations must be keyed on the same RAM number
			// the response reports, for the server's configured runtime.
			wantRecs := catalogdefaults.ForRuntime(runtime, uint64(resp.SystemRamBytes))
			if len(resp.RecommendedOpenModels) != len(wantRecs) {
				t.Errorf("RecommendedOpenModels has %d entries, want %d for %d bytes",
					len(resp.RecommendedOpenModels), len(wantRecs), resp.SystemRamBytes)
			}
			for tier, model := range wantRecs {
				if got := resp.RecommendedOpenModels[tier]; got != model {
					t.Errorf("RecommendedOpenModels[%s] = %q, want %q", tier, got, model)
				}
			}
		})
	}
}

// TestListRuntimeModelsNoManagerReturnsEmpty verifies the no-runtime
// manager path returns an empty response — which older clients must
// read as unknown hardware — rather than fabricating probe results.
func TestListRuntimeModelsNoManagerReturnsEmpty(t *testing.T) {
	srv := NewServer(nil, nil, nil, nil, nil)
	resp, err := srv.ListRuntimeModels(context.Background(), &proto.ListRuntimeModelsRequest{})
	if err != nil {
		t.Fatalf("ListRuntimeModels: %v", err)
	}
	if resp.HardwareOk || resp.MemoryTopology != "" || resp.SystemRamBytes != 0 || len(resp.Gpus) != 0 {
		t.Errorf("no-manager: response should carry no hardware claims, got %+v", resp)
	}
}
