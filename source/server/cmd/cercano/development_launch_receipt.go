package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"cercano/source/server/pkg/agentclient"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/protobuf/proto"
)

func replaceLaunchRecord(path string, s launchSelection) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".launch-operation-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = json.NewEncoder(f).Encode(s); err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func launchReceipt(ctx context.Context, c *agentclient.LifecycleControls, s launchSelection, action string) error {
	if s.LaunchOperation == "" || s.Epoch == "" {
		return fmt.Errorf("unresolved acquisition: preserve %s; retry same operation acquire, never infer epoch", s.OperationFile)
	}
	r := &pb.LifecycleControlRequest{Target: s.Identity, Owner: &pb.LifecycleOwner{Conversation: s.Conversation, Host: s.Identity.Host, Epoch: s.Epoch}, LaunchOperation: s.LaunchOperation, LaunchAction: action}
	response, err := c.LifecycleAcquire(ctx, r)
	if err != nil {
		return err
	}
	if response == nil || !proto.Equal(response.Identity, s.Identity) || !proto.Equal(response.Owner, r.Owner) || response.Receipt != s.LaunchOperation || (action == "deliver" && response.Pending) {
		return fmt.Errorf("ambiguous launch %s response; preserve operation", action)
	}
	return nil
}

// Caller holds current-target.lock. Discovery and selection artifacts have their
// own operation binding: same host/generation is NOT sufficient for rollback.
func cleanupLaunchArtifacts(state string, s launchSelection) error {
	for _, path := range []string{filepath.Join(state, "current-target.json"), filepath.Join(state, "selections", s.Conversation+".json")} {
		var current launchSelection
		err := launchRead(path, &current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			continue
		} // unproven/collision artifact is never ours to delete
		if current.LaunchOperation != s.LaunchOperation || !proto.Equal(current.Identity, s.Identity) {
			continue
		}
		if err = os.Remove(path); err != nil {
			return err
		}
		d, err := os.Open(filepath.Dir(path))
		if err != nil {
			return err
		}
		err = d.Sync()
		d.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
func abortLaunch(state string, s launchSelection) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := launchControls(ctx, s)
	if err != nil {
		return err
	}
	defer c.Close()
	if err = launchReceipt(ctx, c, s, "abort"); err != nil {
		return err
	}
	return cleanupLaunchArtifacts(state, s)
}

// Explicit retry only. Never invoked on timeout/disconnect. Acquire recovery
// queries the SAME durable operation, which the host must resolve idempotently.
func runDevelopmentLaunchReceipt(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("development-launch-receipt", flag.ContinueOnError)
	file := fs.String("operation-file", "", "exact operation journal")
	action := fs.String("action", "", "acquire, deliver, abort")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *action != "acquire" && *action != "deliver" && *action != "abort" {
		return fmt.Errorf("explicit action required")
	}
	var s launchSelection
	if err := launchRead(*file, &s); err != nil {
		return err
	}
	state := filepath.Dir(filepath.Dir(*file))
	if s.OperationFile != *file || s.LaunchOperation == "" || *file != filepath.Join(state, "launch-operations", s.LaunchOperation+".json") {
		return fmt.Errorf("exact canonical operation file required")
	}
	if err := launchPrivateDir(state); err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(state, "current-target.lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if *action == "abort" {
		err = abortLaunch(state, s)
	} else {
		c, e := launchControls(ctx, s)
		if e != nil {
			return e
		}
		defer c.Close()
		if *action == "deliver" {
			err = launchReceipt(ctx, c, s, "deliver")
		} else {
			response, e := c.LifecycleAcquire(ctx, &pb.LifecycleControlRequest{Target: s.Identity, Owner: &pb.LifecycleOwner{Conversation: s.Conversation}, LaunchOperation: s.LaunchOperation, LaunchAction: "acquire"})
			err = e
			if err == nil {
				if response == nil || !proto.Equal(response.Identity, s.Identity) || response.GetOwner().GetConversation() != s.Conversation || response.GetOwner().GetHost() != s.Identity.Host || response.GetOwner().GetEpoch() == "" || response.Receipt != s.LaunchOperation {
					return fmt.Errorf("ambiguous acquire recovery; preserve operation")
				}
				s.Epoch = response.Owner.Epoch
				err = replaceLaunchRecord(*file, s)
			}
		}
	}
	if err != nil {
		return fmt.Errorf("launch %s unresolved; preserve %s and retry --action %s: %w", s.LaunchOperation, *file, *action, err)
	}
	return json.NewEncoder(out).Encode(s)
}
