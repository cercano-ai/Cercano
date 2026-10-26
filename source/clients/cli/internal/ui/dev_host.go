package ui

// Explicit development-only selections. No discovery, launch, or implicit acquisition.
import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/devhostauth"
	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type DevHostSelection struct {
	Endpoint     string
	TokenFile    string
	Identity     *proto.LifecycleIdentity
	Conversation string
	Epoch        string
}

func ReadDevHostSelection(path string) (DevHostSelection, error) {
	var s DevHostSelection
	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(b, &s)
	if err != nil {
		return s, err
	}
	if s.Identity == nil || s.Identity.Host == "" || s.Identity.Build == "" || s.Identity.Contract == "" || s.Identity.Schema <= 0 || s.Identity.Store == "" {
		return s, fmt.Errorf("complete exact development host identity required")
	}
	return s, nil
}
func (s DevHostSelection) Dial(ctx context.Context) (*agentclient.Client, error) {
	if s.Identity == nil {
		return nil, fmt.Errorf("complete exact development host identity required")
	}
	return agentclient.DialAuthenticatedPinnedSession(ctx, agentclient.PinnedSessionIntent{Endpoint: s.Endpoint, HostInstance: s.Identity.Host, Build: s.Identity.Build, Contract: s.Identity.Contract, SchemaVersion: s.Identity.Schema, ConversationID: s.Conversation, OwnershipEpoch: s.Epoch}, s.TokenFile)
}
func (s DevHostSelection) controls(ctx context.Context) (*agentclient.LifecycleControls, error) {
	auth, err := devhostauth.Load(s.Identity.Host, s.TokenFile)
	if err != nil {
		return nil, err
	}
	return agentclient.DialLifecycleControls(ctx, s.Endpoint, func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
		return grpc.NewClient(endpoint, append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, auth.DialOptions()...)...)
	})
}

// Management requests never create an attachment. The server refuses release or
// transfer while ANY terminal is attached. Receipts are returned even on errors.
func ControlDevHost(ctx context.Context, s DevHostSelection, action, destination, handover, receipt string) (*proto.LifecycleControlResponse, *DevHostSelection, error) {
	c, err := s.controls(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer c.Close()
	r := &proto.LifecycleControlRequest{Target: s.Identity, Source: s.Identity, Owner: &proto.LifecycleOwner{Conversation: s.Conversation, Host: s.Identity.Host, Epoch: s.Epoch}, Management: true, HandoverId: handover, Receipt: receipt}
	var out *proto.LifecycleControlResponse
	switch action {
	case "list":
		out, err = c.LifecycleList(ctx, r)
	case "acquire":
		out, err = c.LifecycleAcquire(ctx, r)
	case "release":
		out, err = c.LifecycleRelease(ctx, r)
	case "retire":
		out, err = c.LifecycleRetire(ctx, r)
	case "transfer", "claim":
		d, e := ReadDevHostSelection(destination)
		if e != nil {
			return nil, nil, e
		}
		if handover == "" {
			return nil, nil, fmt.Errorf("explicit unique handover ID required")
		}
		dc, e := d.controls(ctx)
		if e != nil {
			return nil, nil, e
		}
		defer dc.Close()
		r.Destination, r.DestinationEndpoint = d.Identity, d.Endpoint
		if action == "claim" {
			r.Target = d.Identity
			out, err = dc.LifecycleClaim(ctx, r)
		} else {
			out, err = c.Transfer(ctx, dc, r)
		}
		if err == nil && out != nil && !out.Pending && out.Owner != nil {
			d.Conversation, d.Epoch = out.Owner.Conversation, out.Owner.Epoch
			return out, &d, nil
		}
	default:
		err = fmt.Errorf("unknown host action %q", action)
	}
	return out, nil, err
}

type devHostState struct {
	selection DevHostSelection
	version   string
	detached  bool
	busy      bool
}
type devHostResult struct {
	response  *proto.LifecycleControlResponse
	selection *DevHostSelection
	client    *agentclient.Client
	err       error
	action    string
}

func (m Model) WithDevHost(s DevHostSelection, version string) Model {
	m.devHost = &devHostState{selection: s, version: version}
	m.convID = s.Conversation
	m.convRef.id = s.Conversation
	return m
}
func (m Model) runDevHost(line string) (tea.Model, tea.Cmd) {
	f := strings.Fields(line)
	if len(f) < 2 {
		m.mainChat().AppendNotice(&Entry{Role: RoleSystem, Content: "/host detach (retain), reconnect, list, acquire, release, retire, transfer <destination.json> <handover>, claim <destination.json> <handover> <receipt>. Transfer/release require detach first. Exit also retains ownership."})
		m.refreshViewport()
		return m, nil
	}
	state := m.devHost
	if state.busy {
		m.refreshViewport()
		return m, nil
	}
	action := f[1]
	if action == "detach" {
		if m.taskSubscriptionCancel != nil {
			m.taskSubscriptionCancel()
		}
		if m.agent != nil {
			m.agent.Close()
		}
		state.detached = true
		m.mainChat().AppendNotice(&Entry{Role: RoleSystem, Content: "Detached; session retained. /host reconnect validates the same host and epoch."})
		m.refreshViewport()
		return m, nil
	}
	if !state.detached && action != "list" {
		m.mainChat().AppendNotice(&Entry{Role: RoleSystem, Content: "Use /host detach first. Lifecycle management never bypasses live attachments."})
		m.refreshViewport()
		return m, nil
	}
	destination, handover, receipt := "", "", ""
	if len(f) > 2 {
		destination = f[2]
	}
	if len(f) > 3 {
		handover = f[3]
	}
	if len(f) > 4 {
		receipt = f[4]
	}
	state.busy = true
	s := state.selection
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		result := devHostResult{action: action}
		if action == "reconnect" {
			result.client, result.err = s.Dial(ctx)
		} else {
			result.response, result.selection, result.err = ControlDevHost(ctx, s, action, destination, handover, receipt)
		}
		return result
	}
}
func (m Model) applyDevHost(r devHostResult) (tea.Model, tea.Cmd) {
	m.devHost.busy = false
	if r.client != nil {
		// New client-bound slash handlers, same running executable and TUI identity.
		n := New(r.client, false).WithVersion(m.devHost.version).WithDevHost(m.devHost.selection, m.devHost.version)
		// New rebuilds client-bound handlers, but queued subscription messages
		// still belong to this TUI's lifetime. Fence them even before Init's
		// first reducer establishes the replacement subscription.
		n.taskSubscriptionGeneration = m.taskSubscriptionGeneration + 1
		n.width, n.height = m.width, m.height
		return n, tea.Batch(n.Init(), func() tea.Msg { return tea.WindowSizeMsg{Width: m.width, Height: m.height} })
	}
	if r.selection != nil {
		m.devHost.selection = *r.selection
	}
	if r.action == "acquire" && r.err == nil && r.response != nil && r.response.Owner != nil {
		m.devHost.selection.Epoch = r.response.Owner.Epoch
	}
	text := "Host " + r.action
	if r.response != nil {
		b, _ := json.Marshal(r.response)
		text += "\n" + string(b)
	}
	if r.err != nil {
		text += "\nREFUSED / pending (not success): " + r.err.Error()
	} else if r.response != nil && r.response.Pending {
		text += "\nPending; not completed. Preserve the receipt for explicit claim."
	} else {
		text += "\nCompleted; attachment requires explicit /host reconnect."
	}
	m.mainChat().AppendNotice(&Entry{Role: RoleSystem, Content: text})
	m.refreshViewport()
	return m, nil
}
