// Package server — StartClaudeLogin RPC handler.
//
// Server side of the Claude subscription sign-in. The client opens a stream;
// the agent starts the PKCE loopback flow and immediately sends a frame
// carrying the authorize URL for the client to open in a browser, the manual
// URL for a browser on another machine, and a login id. It then blocks on the
// loopback redirect or a code pasted through SubmitClaudeLoginCode under that
// id, whichever comes first. On success it stores the token set in the keychain
// under the profile name, creates/updates a messages+subscription cloud
// profile, optionally activates it, and sends a terminal frame (done=true,
// ok=true). Failures send a terminal frame with ok=false and the reason.
// Mirrors StartChatGPTLogin (the ChatGPT device-auth sibling).
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"

	"cercano/source/server/internal/anthropicauth"
	"cercano/source/server/internal/cloudfactory"
	"cercano/source/server/pkg/config"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// StartClaudeLogin implements proto.AgentServer.
func (s *Server) StartClaudeLogin(req *proto.StartClaudeLoginRequest, stream proto.Agent_StartClaudeLoginServer) error {
	return s.runClaudeLogin(req, stream, false, anthropicauth.Flow{})
}

func (s *Server) runClaudeLogin(req *proto.StartClaudeLoginRequest, stream proto.Agent_StartClaudeLoginServer, reauthenticate bool, flow anthropicauth.Flow) error {
	if err := s.refuseUnintegratedCoexistenceWork(); err != nil {
		return err
	}
	ctx := stream.Context()
	st := s.cfgSvc.Secrets()
	if st == nil {
		return sendClaudeLoginResult(stream, false, "", "keychain unavailable")
	}
	profile := strings.TrimSpace(req.GetProfileName())
	canonicalProfile := profile == ""
	if canonicalProfile {
		profile = "claude"
	}
	model := strings.TrimSpace(req.GetModel())
	modelPinned := model != ""

	np := config.CloudProfile{Name: profile, Flavor: cloudfactory.FlavorMessages, Route: cloudfactory.RouteSubscription, Model: model, ModelPinned: modelPinned}
	login, err := s.cfgSvc.BeginCloudLogin(ctx, np, shouldActivateClaudeLogin(req.GetSetActive(), canonicalProfile), reauthenticate, req.GetCreateOnly())
	if err != nil {
		return sendClaudeLoginResult(stream, false, profile, loginStageFailure("Claude", "profile setup", err))
	}
	defer login.Close()
	ctx = login.Context()

	// Start the loopback authorize and show the user the URL to open.
	pending, err := flow.Start(ctx)
	if err != nil {
		return sendClaudeLoginResult(stream, false, "", loginStageFailure("Claude", "authorization start", err))
	}
	// Own the listener immediately, including failure before Wait is reached.
	defer pending.Close()
	loginID, pasted, release, err := s.registerClaudeLogin(ctx)
	if err != nil {
		return sendClaudeLoginResult(stream, false, "", loginStageFailure("Claude", "authorization start", err))
	}
	defer release()
	if err := stream.Send(&proto.StartClaudeLoginEvent{
		AuthorizeUrl:       pending.AuthorizeURL,
		ManualAuthorizeUrl: pending.ManualAuthorizeURL,
		LoginId:            loginID,
	}); err != nil {
		return err
	}

	// Block until the user approves (or ctx cancels). Wait catches the
	// loopback redirect, renders the success page, and exchanges the code for
	// a token set — or redeems a code pasted through SubmitClaudeLoginCode.
	ts, err := pending.Wait(ctx, pasted)
	if err != nil {
		return sendClaudeLoginResult(stream, false, "", loginStageFailure("Claude", "authorization completion", err))
	}

	encoded, err := ts.Encode()
	if err != nil {
		return sendClaudeLoginResult(stream, false, profile, loginStageFailure("Claude", "credential encoding", err))
	}
	result, err := login.Commit(encoded, ts.Identity)
	if err != nil {
		return sendClaudeLoginResult(stream, false, profile, loginStageFailure("Claude", "credential saving", err))
	}
	if result.Reauthenticated {
		// Credentials may be restored after reset or a failed startup build.
		// Rebind referenced accounts without changing their routing settings.
		if result.Active && s.providerSvc != nil && s.providerSvc.Cloud() == nil {
			if err := s.rebuildCloud(); err != nil {
				s.persistConfig()
				return sendClaudeLoginResult(stream, false, profile, loginStageFailure("Claude", "provider activation", err))
			}
		}
		s.persistConfig()
		return sendClaudeLoginResult(stream, true, profile, "")
	}
	isActive := result.Active
	np = result.Profile

	if isActive {
		if err := s.rebuildCloud(); err != nil {
			s.persistConfig()
			return sendClaudeLoginResult(stream, false, profile, loginStageFailure("Claude", "provider activation", err))
		}
		s.broadcastConfigChanged("active_cloud_profile", profile)
		s.broadcastConfigChanged("cloud_model", np.Model)
	}
	s.persistConfig()
	return sendClaudeLoginResult(stream, true, profile, "")
}

// shouldActivateClaudeLogin preserves the explicit set_active request while
// making the canonical no-profile "sign in with Claude" path activate by
// default. Older/stale clients can omit set_active and still get the onboarding
// behavior users expect; explicit named-profile reauth remains non-activating
// unless requested.
func shouldActivateClaudeLogin(setActive, canonicalProfile bool) bool {
	return setActive || canonicalProfile
}

// sendClaudeLoginResult emits the terminal frame of the sign-in stream.
func sendClaudeLoginResult(stream proto.Agent_StartClaudeLoginServer, ok bool, profile, errMsg string) error {
	return stream.Send(&proto.StartClaudeLoginEvent{
		Done:        true,
		Ok:          ok,
		ProfileName: profile,
		Error:       errMsg,
	})
}

// claudeLogin is the paste inbox of one in-flight sign-in, keyed by login id
// in Server.claudeLogins. done closes when the sign-in stops waiting, so a
// submit never blocks on an inbox nobody reads.
type claudeLogin struct {
	pasted chan anthropicauth.PastedCode
	done   <-chan struct{}
}

// registerClaudeLogin opens a paste inbox under a fresh unguessable login id.
// release must be called once the sign-in stops waiting.
func (s *Server) registerClaudeLogin(ctx context.Context) (string, <-chan anthropicauth.PastedCode, func(), error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", nil, nil, err
	}
	id := hex.EncodeToString(b[:])
	ctx, cancel := context.WithCancel(ctx)
	l := &claudeLogin{pasted: make(chan anthropicauth.PastedCode), done: ctx.Done()}
	s.claudeLogins.Store(id, l)
	return id, l.pasted, func() {
		s.claudeLogins.Delete(id)
		cancel()
	}, nil
}

// SubmitClaudeLoginCode implements proto.AgentServer. It hands a pasted code
// to the waiting sign-in and reports whether it was accepted; the sign-in's
// own stream carries the terminal frame either way.
func (s *Server) SubmitClaudeLoginCode(ctx context.Context, req *proto.SubmitClaudeLoginCodeRequest) (*proto.SubmitClaudeLoginCodeResponse, error) {
	v, ok := s.claudeLogins.Load(req.GetLoginId())
	if !ok {
		return nil, status.Error(codes.NotFound, "no Claude sign-in is waiting under that id")
	}
	l := v.(*claudeLogin)
	result := make(chan error, 1)
	select {
	case l.pasted <- anthropicauth.PastedCode{Code: req.GetCode(), Result: result}:
	case <-l.done:
		return nil, status.Error(codes.FailedPrecondition, "the Claude sign-in is no longer waiting")
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	select {
	case err := <-result:
		if err != nil {
			return &proto.SubmitClaudeLoginCodeResponse{Error: pastedCodeFailure(err)}, nil
		}
		return &proto.SubmitClaudeLoginCodeResponse{Accepted: true}, nil
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	}
}

// pastedCodeFailure explains a rejected paste in terms the user can act on,
// keeping loginFailure's no-raw-detail rule for everything else.
func pastedCodeFailure(err error) string {
	if errors.Is(err, anthropicauth.ErrInvalidPastedCode) {
		return "That isn't a code for this sign-in. Copy the whole code shown after approving, and paste it again."
	}
	return loginFailure(err)
}
