package ui

// Reopening is an explicit user action, never a side effect of publication or
// connection loss. The lifecycle service remains the authority for busy refusal.
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"cercano/source/server/pkg/proto"
	tea "charm.land/bubbletea/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	protobuf "google.golang.org/protobuf/proto"
)

func reopenDevHost(ctx context.Context, destination DevHostSelection, conversation string) (DevHostSelection, error) {
	dc, err := destination.controls(ctx)
	if err != nil {
		return destination, err
	}
	defer dc.Close()
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(destination.Identity.Store), "hosts", "*.json"))
	if err != nil {
		return destination, err
	}
	source, owner, err := findReopenOwner(paths, conversation, func(path string) (DevHostSelection, *proto.LifecycleControlResponse, error) {
		// A dead unrelated endpoint must not exhaust the whole reopen budget.
		ctx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		st, err := os.Lstat(path)
		if err != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			return DevHostSelection{}, nil, fmt.Errorf("cannot read private host record")
		}
		source, err := ReadDevHostSelection(path)
		if err != nil {
			return source, nil, err
		}
		if source.Identity.Store != destination.Identity.Store || source.Identity.Contract != destination.Identity.Contract || source.Identity.Schema != destination.Identity.Schema {
			return source, nil, fmt.Errorf("incompatible host record")
		}
		sc, err := source.controls(ctx)
		if err != nil {
			return source, nil, err
		}
		defer sc.Close()
		listing, err := sc.LifecycleList(ctx, &proto.LifecycleControlRequest{Target: source.Identity, Management: true})
		return source, listing, err
	})
	var unavailable *reopenDiscoveryUnavailable
	if err != nil && !errors.As(err, &unavailable) {
		return destination, err
	}
	if owner != nil {
		if protobuf.Equal(source.Identity, destination.Identity) {
			destination.Conversation, destination.Epoch = conversation, owner.Epoch
			return destination, nil
		}
		sc, err := source.controls(ctx)
		if err != nil {
			return destination, err
		}
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			sc.Close()
			return destination, err
		}
		result, err := sc.Transfer(ctx, dc, &proto.LifecycleControlRequest{Target: source.Identity, Source: source.Identity, Destination: destination.Identity, DestinationEndpoint: destination.Endpoint, Owner: owner, Management: true, HandoverId: hex.EncodeToString(nonce[:])})
		sc.Close()
		if err != nil || result.GetPending() || result.GetOwner() == nil {
			if result.GetReceipt() != "" {
				return destination, fmt.Errorf("reopen was not completed; handover needs explicit recovery; no retry scheduled: %v", err)
			}
			return destination, fmt.Errorf("conversation remains on its original host; outstanding work or attached clients must finish first. Work is preserved; no transfer is scheduled: %v", err)
		}
		destination.Conversation, destination.Epoch = conversation, result.Owner.Epoch
		return destination, nil
	}
	result, err := dc.LifecycleAcquire(ctx, &proto.LifecycleControlRequest{Target: destination.Identity, Owner: &proto.LifecycleOwner{Conversation: conversation}})
	if err != nil {
		if unavailable != nil {
			return destination, fmt.Errorf("owner discovery incomplete (%v); authoritative acquisition refused: %w", unavailable, err)
		}
		return destination, err
	}
	if result.GetPending() || !protobuf.Equal(result.GetIdentity(), destination.Identity) || result.GetOwner().GetConversation() != conversation || result.GetOwner().GetHost() != destination.Identity.Host || result.GetOwner().GetEpoch() == "" {
		return destination, fmt.Errorf("reopen returned no ownership")
	}
	destination.Conversation, destination.Epoch = conversation, result.Owner.Epoch
	return destination, nil
}

// A positive retained owner is authoritative under the store's exclusive ownership
// contract. Scan all reachable hosts to reject conflicting live claims. With no
// positive owner, unavailable listings are not ownership evidence. The destination
// may still attempt ordinary acquisition: the store lock AND durable owned/reserved
// record reject takeover. Only new or explicitly released ownership can succeed.
type reopenDiscoveryUnavailable struct{ cause error }

func (e *reopenDiscoveryUnavailable) Error() string { return e.cause.Error() }
func (e *reopenDiscoveryUnavailable) Unwrap() error { return e.cause }

func findReopenOwner(paths []string, conversation string, list func(string) (DevHostSelection, *proto.LifecycleControlResponse, error)) (DevHostSelection, *proto.LifecycleOwner, error) {
	var selected DevHostSelection
	var found *proto.LifecycleOwner
	var uncertain error
	for _, path := range paths {
		source, listing, err := list(path)
		if err != nil {
			if status.Code(err) != codes.Unavailable && status.Code(err) != codes.DeadlineExceeded && !errors.Is(err, context.DeadlineExceeded) {
				return selected, nil, err
			}
			uncertain = err
			continue
		}
		if listing == nil || !protobuf.Equal(listing.Identity, source.Identity) {
			return selected, nil, fmt.Errorf("host identity mismatch")
		}
		for _, owner := range listing.Retained {
			if owner.Conversation != conversation {
				continue
			}
			if owner.Host != source.Identity.Host || owner.Epoch == "" {
				return selected, nil, fmt.Errorf("invalid retained owner")
			}
			if found != nil {
				return selected, nil, fmt.Errorf("ambiguous retained ownership")
			}
			selected, found = source, owner
		}
	}
	if found == nil && uncertain != nil {
		return selected, nil, &reopenDiscoveryUnavailable{cause: uncertain}
	}
	return selected, found, nil
}

func (m Model) beginDevHostReopen(conversation string) (Model, tea.Cmd) {
	if m.devHost.busy || m.devHost.detached {
		m.errMsg = "Reopen unavailable while disconnected or another reopen is running"
		return m, nil
	}
	m.devHost.busy = true
	selection := m.devHost.selection
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		s, err := reopenDevHost(ctx, selection, conversation)
		r := devHostResult{action: "reopen", err: err}
		if err == nil {
			r.selection = &s
			r.client, r.err = s.Dial(ctx)
		}
		return r
	}
}

// Fresh IDs need no discovery or transfer: acquire only on the exact current host.
func (m Model) beginDevHostFreshConversation() (Model, tea.Cmd) {
	if m.devHost.busy || m.devHost.detached {
		m.errMsg = "Clear unavailable while disconnected or another lifecycle operation is running"
		return m, nil
	}
	m.devHost.busy = true
	selection := m.devHost.selection
	conversation := newConvID()
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		r := devHostResult{action: "reopen"}
		c, err := selection.controls(ctx)
		if err != nil {
			r.err = err
			return r
		}
		defer c.Close()
		response, err := c.LifecycleAcquire(ctx, &proto.LifecycleControlRequest{Target: selection.Identity, Owner: &proto.LifecycleOwner{Conversation: conversation}})
		if err != nil {
			r.err = err
			return r
		}
		owner := response.GetOwner()
		if response.GetPending() || owner.GetConversation() != conversation || owner.GetHost() != selection.Identity.Host || owner.GetEpoch() == "" {
			r.err = fmt.Errorf("fresh acquisition returned invalid ownership")
			return r
		}
		selection.Conversation, selection.Epoch = conversation, owner.Epoch
		r.selection = &selection
		r.client, r.err = selection.Dial(ctx)
		return r
	}
}
