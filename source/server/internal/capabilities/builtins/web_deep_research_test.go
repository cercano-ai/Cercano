package builtins

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cercano/source/server/internal/capabilities"
)

func TestDeepResearchWriteTier(t *testing.T) {
	if got := DeepResearch().Tier(); got != capabilities.TierW {
		t.Fatalf("tier = %v, want W: all research persists output", got)
	}
}

func TestDeepResearchWriteApproval(t *testing.T) {
	for _, output := range []string{"", "reports", "/absolute/research"} {
		for _, mode := range []string{"deny", "unavailable", "error"} {
			t.Run(output+"/"+mode, func(t *testing.T) {
				work := t.TempDir()
				explicit := output
				if filepath.IsAbs(output) {
					explicit = filepath.Join(work, "absolute", "research")
				}
				want := explicit
				if want == "" {
					want = filepath.Join(work, "scratch", "research", "test-topic")
				} else if !filepath.IsAbs(want) {
					want = filepath.Join(work, want)
				}
				args, _ := json.Marshal(deepResearchArgs{Topic: "Test Topic", Intent: "test", OutputDir: explicit})
				call := &capabilities.Call{Args: args, WorkDir: work}
				calls := 0
				sentinel := errors.New("approval unavailable")
				if mode != "unavailable" {
					call.RequestPermission = func(_ context.Context, reason string) (bool, error) {
						calls++
						if !strings.Contains(reason, want) {
							t.Errorf("approval reason %q missing actual path %q", reason, want)
						}
						if mode == "error" {
							return false, sentinel
						}
						return false, nil
					}
				}
				result, err := DeepResearch().Execute(context.Background(), call)
				if result != nil || err == nil || !strings.Contains(err.Error(), "permission") {
					t.Fatalf("result=%v err=%v, want permission failure", result, err)
				}
				if mode == "error" && !errors.Is(err, sentinel) {
					t.Errorf("lost permission error: %v", err)
				}
				if mode != "unavailable" && calls != 1 {
					t.Errorf("approval calls = %d, want 1", calls)
				}
				entries, err := os.ReadDir(work)
				if err != nil || len(entries) != 0 {
					t.Fatalf("denial created output: %v, %v", entries, err)
				}
			})
		}
	}
}

func TestDeepResearchApprovedOutput(t *testing.T) {
	for _, output := range []string{"", "reports", "/absolute/research"} {
		t.Run(output, func(t *testing.T) {
			work := t.TempDir()
			want := output
			if want == "" {
				want = filepath.Join(work, "scratch", "research", "test-topic")
			} else if !filepath.IsAbs(want) {
				want = filepath.Join(work, want)
			}
			calls := 0
			call := &capabilities.Call{WorkDir: work, RequestPermission: func(_ context.Context, reason string) (bool, error) {
				calls++
				if !strings.Contains(reason, want) {
					t.Errorf("reason %q missing %q", reason, want)
				}
				return true, nil
			}}
			got, err := approveDeepResearchOutput(context.Background(), call, deepResearchArgs{Topic: "Test Topic", OutputDir: output})
			if err != nil || got != want || calls != 1 {
				t.Fatalf("approved path=%q err=%v calls=%d; want %q", got, err, calls, want)
			}
			entries, err := os.ReadDir(work)
			if err != nil || len(entries) != 0 {
				t.Fatalf("approval itself created files: %v %v", entries, err)
			}
		})
	}
}

func TestDeepResearchApprovalNormalizesRelativeWorkDir(t *testing.T) {
	for _, work := range []string{"", "workspace"} {
		for _, output := range []string{"", "reports"} {
			t.Run(work+"/"+output, func(t *testing.T) {
				want := filepath.Join(work, "scratch", "research", "test-topic")
				if output != "" {
					want = filepath.Join(work, output)
				}
				want, err := filepath.Abs(want)
				if err != nil {
					t.Fatal(err)
				}
				calls := 0
				call := &capabilities.Call{WorkDir: work, RequestPermission: func(_ context.Context, reason string) (bool, error) {
					calls++
					if !strings.Contains(reason, want) {
						t.Errorf("approval reason %q missing absolute path %q", reason, want)
					}
					return true, nil
				}}
				got, err := approveDeepResearchOutput(context.Background(), call, deepResearchArgs{Topic: "Test Topic", OutputDir: output})
				if err != nil || got != want || calls != 1 {
					t.Fatalf("approved path=%q err=%v calls=%d; want %q", got, err, calls, want)
				}
			})
		}
	}
}
