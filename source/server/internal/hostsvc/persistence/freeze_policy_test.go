package persistence

import (
	"context"
	"fmt"
	"testing"

	"cercano/source/server/internal/compactiongen"
	"cercano/source/server/internal/compactor"
	"cercano/source/server/internal/conversation"
	configsvc "cercano/source/server/internal/hostsvc/config"
	"cercano/source/server/internal/secrets"
	cfg "cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type freezeHistoryStore struct{ saves int }

func (*freezeHistoryStore) GetTurns(context.Context, string) ([]conversation.Turn, error) {
	return nil, nil
}
func (*freezeHistoryStore) GetCompaction(context.Context, string) (conversation.Compaction, error) {
	return conversation.Compaction{}, nil
}
func (s *freezeHistoryStore) SaveCompaction(context.Context, conversation.Compaction) error {
	s.saves++
	return nil
}

type freezeProgressStream struct {
	proto.Agent_RegenerateContextServer
	last *proto.RegenerateContextProgress
}

func (*freezeProgressStream) Context() context.Context                        { return context.Background() }
func (s *freezeProgressStream) Send(p *proto.RegenerateContextProgress) error { s.last = p; return nil }

func TestHistoryFreezeIndependentOfCredentials(t *testing.T) {
	for _, frozen := range []bool{false, true} {
		for _, disabled := range []string{"0", "1"} {
			t.Run(fmt.Sprintf("frozen=%t/credentials-disabled=%s", frozen, disabled), func(t *testing.T) {
				t.Setenv("CERCANO_DISABLE_CREDENTIALS", disabled)
				x := &svc{cfgSvc: configsvc.New("", cfg.Config{}, secrets.NewMemory(), configsvc.Options{Frozen: frozen})}
				history := &freezeHistoryStore{}
				g := compactiongen.New(history, nil, compactor.Config{}, nil, 0)
				t.Cleanup(func() {
					if err := g.Close(context.Background()); err != nil {
						t.Error(err)
					}
				})
				if got := x.cfgSvc.Frozen(); got != frozen {
					t.Fatalf("Frozen()=%t, want %t", got, frozen)
				}
				x.SetCompactionGenerator(g)
				stream := &freezeProgressStream{}
				err := x.RegenerateContext(&proto.RegenerateContextRequest{ConversationId: "history", ClearOnly: true}, stream)
				if frozen {
					if status.Code(err) != codes.FailedPrecondition || x.compactionGen != nil || history.saves != 0 || stream.last != nil {
						t.Fatalf("frozen operation: err=%v generator=%p saves=%d progress=%v", err, x.compactionGen, history.saves, stream.last)
					}
				} else if err != nil || history.saves != 1 || stream.last == nil || !stream.last.Ok {
					t.Fatalf("ordinary history clear: err=%v saves=%d progress=%v", err, history.saves, stream.last)
				}
			})
		}
	}
}
