package setupresetcmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"cercano/source/server/internal/secrets"
	"cercano/source/server/internal/setupreset"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrNoAgent = errors.New("no agent is listening")

type Dependencies struct {
	ConfigPath string
	WizardPath string
	OpenStore  func() (secrets.Store, error)
	// TryAgent must return ErrNoAgent only when no agent is running for the
	// profile. Never fall back locally after an RPC may already have
	// performed the reset.
	TryAgent func(context.Context) (Outcome, error)
}

func Perform(ctx context.Context, d Dependencies) (Outcome, error) {
	var outcome Outcome
	if err := ctx.Err(); err != nil {
		return outcome, err
	}
	// Environment-overridden wizard paths must never replace the config itself.
	if filepath.Clean(d.ConfigPath) == filepath.Clean(d.WizardPath) {
		return outcome, fmt.Errorf("config and wizard paths must be different files")
	}
	configInfo, _ := os.Stat(d.ConfigPath)
	wizardInfo, _ := os.Stat(d.WizardPath)
	if configInfo != nil && wizardInfo != nil && os.SameFile(configInfo, wizardInfo) {
		return outcome, fmt.Errorf("config and wizard paths refer to the same file")
	}
	if _, err := setupreset.Prepare(d.ConfigPath); err != nil {
		return outcome, err
	}
	err := ErrNoAgent
	if d.TryAgent != nil {
		outcome, err = d.TryAgent(ctx)
	}
	if errors.Is(err, ErrNoAgent) {
		if d.OpenStore == nil {
			return outcome, fmt.Errorf("Cercano credential store is unavailable")
		}
		store, openErr := d.OpenStore()
		if openErr != nil {
			return outcome, fmt.Errorf("open Cercano credential store: %w", openErr)
		}
		result, resetErr := setupreset.Reset(ctx, d.ConfigPath, store, nil)
		outcome = Outcome{CredentialsRemoved: result.DeletedCredentials, ConfigWritten: result.ConfigWritten}
		err = resetErr
	}
	if err != nil {
		return outcome, err
	}
	if err = setupreset.WriteFreshWizard(d.WizardPath); err != nil {
		return outcome, fmt.Errorf("settings reset, but fresh wizard state could not be written: %w", err)
	}
	outcome.WizardWritten = true
	return outcome, nil
}

// TryProfileAgent resets through profile's running agent and never starts
// one. With no agent answering for the profile it returns ErrNoAgent, so the
// reset runs locally. An agent that is registered but does not answer in time
// may still be running, so that is an error rather than a local reset.
func TryProfileAgent(profile string) func(context.Context) (Outcome, error) {
	return func(ctx context.Context) (Outcome, error) {
		var outcome Outcome
		client, err := dialProfileExisting(ctx, profile)
		if err != nil {
			if errors.Is(err, agentclient.ErrNoProfileAgent) && status.Code(err) != codes.DeadlineExceeded {
				return outcome, ErrNoAgent
			}
			return outcome, fmt.Errorf("contact the agent for profile %q: %w", profile, err)
		}
		defer client.Close()
		outcome.UsedAgent = true
		callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		result, err := proto.NewAgentClient(client.Conn()).ResetSetup(callCtx, &proto.ResetSetupRequest{Confirmed: true})
		if err != nil {
			if status.Code(err) == codes.Unimplemented {
				return outcome, fmt.Errorf("the running agent does not support setup reset; update/restart that agent and let sessions reconnect before retrying")
			}
			outcome.ResultUnknown = true
			return outcome, fmt.Errorf("agent reset failed or its result is unknown; no local fallback was attempted: %w", err)
		}
		outcome.CredentialsRemoved = int(result.GetCredentialsRemoved())
		outcome.ConfigWritten = result.GetConfigWritten()
		outcome.LiveApplied = result.GetLiveApplied()
		outcome.Warning = result.GetWarning()
		if !result.GetOk() {
			return outcome, fmt.Errorf("%s", result.GetError())
		}
		return outcome, nil
	}
}

// dialProfileExisting connects to a profile's running agent; tests may
// replace it.
var dialProfileExisting = agentclient.DialProfileExisting
