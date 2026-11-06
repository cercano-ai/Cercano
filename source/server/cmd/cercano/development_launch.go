package main

// This orchestration path never enters normal singleton startup. A durable start
// intent prevents replacement even when a child fails before publishing readiness.
import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"cercano/source/server/internal/conversation"
	"cercano/source/server/pkg/agentclient"
	"cercano/source/server/pkg/devhostauth"
	pb "cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

type launchSelection struct {
	LaunchOperation string `json:",omitempty"`
	OperationFile   string `json:",omitempty"`
	// Fixture restricts synthetic dependency-factory terminals only; never authorization.
	Fixture         bool `json:",omitempty"`
	ConfigAuthority string
	Endpoint        string
	TokenFile       string
	Identity        *pb.LifecycleIdentity
	Conversation    string
	Epoch           string
	Generation      string
}

func launchPrivateDir(path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(path) || resolved != path {
		return fmt.Errorf("canonical absolute directory required: %s", path)
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	owned, ownErr := devOwnedByCaller(st)
	if ownErr != nil {
		return ownErr
	}
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 || !owned {
		return fmt.Errorf("private owned directory required: %s", path)
	}
	return nil
}
func launchID() (string, error) {
	b := make([]byte, 16)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func launchExclusive(path string, value interface{}) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = json.NewEncoder(f).Encode(value); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	dir, e := os.Open(filepath.Dir(path))
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
func launchControls(ctx context.Context, s launchSelection) (*agentclient.LifecycleControls, error) {
	a, e := devhostauth.Load(s.Identity.Host, s.TokenFile)
	if e != nil {
		return nil, e
	}
	return agentclient.DialLifecycleControls(ctx, s.Endpoint, func(ctx context.Context, endpoint string) (*grpc.ClientConn, error) {
		return grpc.NewClient(endpoint, append([]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, a.DialOptions()...)...)
	})
}
func launchPrivateFile(path string) error {
	st, e := os.Lstat(path)
	if e != nil {
		return e
	}
	owned, ownErr := devOwnedByCaller(st)
	if ownErr != nil {
		return ownErr
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || !owned {
		return fmt.Errorf("private owned regular file required: %s", path)
	}
	return nil
}
func launchRead(path string, value interface{}) error {
	if e := launchPrivateFile(path); e != nil {
		return e
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, value)
}

type developmentBlockedRecovery func(context.Context, string, string, string, launchSelection, error) error

func runDevelopmentLaunch(ctx context.Context, args []string, out io.Writer) error {
	return runDevelopmentLaunchWithRecovery(ctx, args, out, func(ctx context.Context, base, generation, authority string, s launchSelection, cause error) error {
		return developmentReplaceBlocked(ctx, base, generation, authority, s, cause, developmentConfirm)
	})
}

func runDevelopmentLaunchWithRecovery(ctx context.Context, args []string, out io.Writer, recoverBlocked developmentBlockedRecovery) error {
	return runDevelopmentLaunchWithHooks(ctx, args, out, recoverBlocked, developmentHostCommand)
}

type developmentLaunchCommand func(string, string, string, string, string, string, string, io.Writer, ...string) *exec.Cmd

func runDevelopmentLaunchWithHooks(ctx context.Context, args []string, out io.Writer, recoverBlocked developmentBlockedRecovery, hostCommand developmentLaunchCommand) error {
	fs := flag.NewFlagSet("development-launch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	installed := fs.Bool("installed", false, "select this installed agent keg; still requires prepared state")
	handoff := fs.Bool("launch-handoff", false, "pending handoff to CLI; caller must explicitly abort definitive pre-exec failures")
	authority := fs.String("config-authority", "", "absolute isolated config authority shared across generations")
	state := fs.String("state", "", "prepared private state")
	generation := fs.String("generation", "", "exact published generation")
	runtime := fs.String("runtime-root", "", "short private runtime root")
	if e := fs.Parse(args); e != nil {
		return e
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	for _, p := range []string{*state, *runtime} {
		if e := launchPrivateDir(p); e != nil {
			return e
		}
	}
	if len([]byte(*runtime)) > 38 {
		return fmt.Errorf("runtime-root exceeds 38 resolved bytes")
	}
	var installedBinary, installedBuild string
	if *installed {
		if *generation != "" {
			return fmt.Errorf("installed launch selects its own keg; --generation refused")
		}
		var err error
		*generation, installedBinary, installedBuild, err = installedGeneration()
		if err != nil {
			return err
		}
	}
	resolved, e := filepath.EvalSymlinks(*generation)
	if e != nil {
		return e
	}
	if !filepath.IsAbs(*generation) || resolved != *generation {
		return fmt.Errorf("canonical absolute generation required")
	}
	contract := conversation.DevelopmentCoexistenceDeclaration()
	store := filepath.Join(*state, "conversations.db")
	configAuthority, e := developmentConfigAuthority(*state, *authority)
	if e != nil {
		return e
	}
	for _, p := range []string{store, configAuthority} {
		if e = launchPrivateFile(p); e != nil {
			return fmt.Errorf("prepared private database and explicit config.yaml required; provision once with prepare-development --new-state /absolute/NEW/state (no legacy import): %w", e)
		}
	}
	if e = conversation.PreflightCoexistence(ctx, store, contract); e != nil {
		return fmt.Errorf("prepared development store required (provision once with prepare-development --new-state NEW; never migrate legacy data): %w", e)
	}
	var manifest struct {
		Build    string `json:"build_id"`
		Binaries map[string]struct {
			SHA256 string `json:"sha256"`
		}
	}
	binary := installedBinary
	if *installed {
		manifest.Build = installedBuild
	} else {
		b, e := os.ReadFile(filepath.Join(*generation, "manifest.json"))
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &manifest); e != nil {
			return e
		}
		binary = filepath.Join(*generation, "cercano")
		self, e := os.Executable()
		if e != nil {
			return e
		}
		self, e = filepath.EvalSymlinks(self)
		if e != nil {
			return e
		}
		if self != binary {
			return fmt.Errorf("development-launch must execute exact generation/cercano")
		}
		for _, name := range []string{"cercano", "cercano-cli"} {
			b, e = os.ReadFile(filepath.Join(*generation, name))
			if e != nil {
				return e
			}
			sum := sha256.Sum256(b)
			if manifest.Build == "" || hex.EncodeToString(sum[:]) != manifest.Binaries[name].SHA256 {
				return fmt.Errorf("generation binary digest mismatch: %s", name)
			}
		}
	}
	hosts := filepath.Join(*state, "hosts")
	selections := filepath.Join(*state, "selections")
	for _, p := range []string{hosts, selections, filepath.Join(*state, "launch-operations")} {
		if e = os.Mkdir(p, 0700); e != nil && !os.IsExist(e) {
			return e
		}
		if e = launchPrivateDir(p); e != nil {
			return e
		}
	}
	// Serialize target publication across generations, not only per-host starts.
	targetLock, e := os.OpenFile(filepath.Join(*state, "current-target.lock"), os.O_CREATE|os.O_RDWR|devOpenNoFollow, 0600)
	if e != nil {
		return e
	}
	defer targetLock.Close()
	if e = launchLock(ctx, targetLock); e != nil {
		return e
	}
	defer devFlock(targetLock, devLockUnlock)
	key := sha256.Sum256([]byte(*generation))
	base := filepath.Join(hosts, hex.EncodeToString(key[:]))
	lock, e := os.OpenFile(base+".lock", os.O_CREATE|os.O_RDWR|devOpenNoFollow, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = launchLock(ctx, lock); e != nil {
		return e
	}
	defer devFlock(lock, devLockUnlock)
	generationBase := base
	base, e = developmentInstanceBase(generationBase)
	if e != nil {
		return e
	}
	freshHost := ""
	replaced := false
	for {
		var instanceLock *os.File
		if base != generationBase {
			instanceLock, e = developmentInstanceLock(base)
			if e != nil {
				return e
			}
			defer instanceLock.Close()
		}
		record := base + ".json"
		var s launchSelection
		e = launchRead(record, &s)
		if os.IsNotExist(e) {
			host := freshHost
			if host == "" {
				var intent map[string]string
				intentErr := launchRead(base+".starting", &intent)
				if intentErr == nil {
					if replaced {
						return fmt.Errorf("replacement attempt did not become ready; intent retained")
					}
					if e = recoverBlocked(ctx, base, *generation, configAuthority, s, fmt.Errorf("prior start intent exists")); e != nil {
						return e
					}
					base, freshHost, e = developmentNewInstance(generationBase, *generation, configAuthority)
					if e != nil {
						return e
					}
					replaced = true
					continue
				}
				if !os.IsNotExist(intentErr) {
					return intentErr
				}
				host, e = launchID()
				if e != nil {
					return e
				}
				if e = launchExclusive(base+".starting", map[string]string{"host": host, "generation": *generation, "config_authority": configAuthority}); e != nil {
					return e
				}
			}
			token := base + ".token"
			if e = devhostauth.Create(token); e != nil {
				return e
			}
			log, e := os.OpenFile(base+".log", os.O_CREATE|os.O_APPEND|os.O_WRONLY|devOpenNoFollow, 0600)
			if e != nil {
				return e
			}
			cmd := hostCommand(binary, *state, *generation, host, token, record, *runtime, log, configAuthority)
			if *installed {
				cmd.Args = append(cmd.Args, "--installed")
			}
			e = cmd.Start()
			log.Close()
			if e != nil {
				return e
			}
			_ = cmd.Process.Release()
			deadline := time.Now().Add(30 * time.Second)
			for {
				e = launchRead(record, &s)
				if e == nil {
					break
				}
				if !os.IsNotExist(e) || time.Now().After(deadline) {
					return fmt.Errorf("host not ready; start intent retained, no replacement: %w", e)
				}
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(100 * time.Millisecond):
				}
			}
			if s.Identity == nil || s.Identity.Host != host || s.TokenFile != token {
				return fmt.Errorf("spawned host identity mismatch")
			}
		} else if e != nil {
			return e
		}
		expected := &pb.LifecycleIdentity{Host: s.Identity.GetHost(), Build: manifest.Build, Contract: contract.Contract, Schema: int32(contract.SchemaVersion), Store: store}
		if e := s.validateConfigAuthority(configAuthority); e != nil {
			return e
		}
		if expected.Host == "" || !proto.Equal(s.Identity, expected) || s.Conversation != "" || s.Epoch != "" || s.TokenFile != base+".token" {
			return fmt.Errorf("host record identity mismatch")
		}
		ready, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		controls, e := launchControls(ready, s)
		var check *pb.LifecycleControlResponse
		if e == nil {
			defer controls.Close()
			check, e = controls.LifecycleList(ready, &pb.LifecycleControlRequest{Target: expected, Management: true})
		}
		if e != nil {
			if controls != nil {
				controls.Close()
			}
			cancel()
			if replaced {
				return fmt.Errorf("replacement host unavailable; intent retained: %w", e)
			}
			if e = recoverBlocked(ctx, base, *generation, configAuthority, s, e); e != nil {
				return e
			}
			base, freshHost, e = developmentNewInstance(generationBase, *generation, configAuthority)
			if e != nil {
				return e
			}
			replaced = true
			continue
		}
		if !proto.Equal(check.Identity, expected) {
			return fmt.Errorf("authenticated readiness identity mismatch")
		}
		id, e := launchID()
		if e != nil {
			return e
		}
		operation, e := launchID()
		if e != nil {
			return e
		}
		s.Conversation, s.Generation, s.LaunchOperation = id, *generation, operation
		s.OperationFile = filepath.Join(*state, "launch-operations", operation+".json")
		if e = launchExclusive(s.OperationFile, s); e != nil {
			return e
		}
		// Sync before acquisition: lost responses must have a recoverable ID.
		if e = replaceLaunchRecord(s.OperationFile, s); e != nil {
			return e
		}
		acquired, e := controls.LifecycleAcquire(ready, &pb.LifecycleControlRequest{Target: expected, Owner: &pb.LifecycleOwner{Conversation: id}, LaunchOperation: operation, LaunchAction: "acquire"})
		if e != nil {
			return fmt.Errorf("ambiguous launch acquisition; do not release or replace host; retry development-launch-receipt --operation-file %s --action acquire: %w", s.OperationFile, e)
		}
		if acquired == nil || !proto.Equal(acquired.Identity, expected) || acquired.GetOwner().GetConversation() != id || acquired.GetOwner().GetHost() != expected.Host || acquired.GetOwner().GetEpoch() == "" || acquired.Receipt != operation {
			return fmt.Errorf("ambiguous acquire identity/receipt; preserve %s; retry same operation acquire", s.OperationFile)
		}
		s.Epoch = acquired.Owner.Epoch
		compensate := func(cause error) error {
			if err := abortLaunch(*state, s); err != nil {
				return fmt.Errorf("%w; cleanup unresolved: %v; retry development-launch-receipt --operation-file %s --action abort (first --action acquire if epoch was not saved)", cause, err, s.OperationFile)
			}
			return cause
		}
		if e = replaceLaunchRecord(s.OperationFile, s); e != nil {
			return compensate(e)
		}
		selection := filepath.Join(selections, id+".json")
		if e = launchExclusive(selection, s); e != nil {
			return compensate(e)
		}
		if e = publishCurrentTarget(*state, s); e != nil {
			return compensate(e)
		}
		if !*handoff {
			// Direct output is a delivery, not permission to cancel later. Claim
			// before exposing details; a partial/lost output must retain ownership.
			if e = launchReceipt(ready, controls, s, "deliver"); e != nil {
				return fmt.Errorf("delivery ambiguous; preserve %s; do not abort: %w", s.OperationFile, e)
			}
		}
		n, e := fmt.Fprintln(out, selection)
		if e != nil && *handoff && n == 0 {
			return compensate(e)
		}
		if e != nil {
			return fmt.Errorf("output ambiguous or already delivered; retain %s, do not abort: %w", s.OperationFile, e)
		}
		return nil
	}
}

// developmentHostCommand is the runtime spawn boundary, after launch validation.
func developmentHostCommand(binary, state, generation, host, token, record, runtime string, log io.Writer, configAuthority ...string) *exec.Cmd {
	cmd := exec.Command(binary, "development-host", "--state", state, "--generation", generation, "--host", host, "--token-file", token, "--record", record, "--runtime-root", runtime, "--listen", "127.0.0.1:0")
	if len(configAuthority) != 0 {
		cmd.Args = append(cmd.Args, "--config-authority", configAuthority[0])
	}
	// Runtime inherits caller policy, including absence and malformed values.
	// Only build subprocesses force credentials off; validation remains in the host.
	cmd.Env = os.Environ()
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = devDetachSysProcAttr()
	return cmd
}

// Refuse old/unbound records too; never silently repurpose a running host.
func (s launchSelection) validateConfigAuthority(want string) error {
	if want == "" || s.ConfigAuthority != want {
		return fmt.Errorf("host config authority mismatch; refusing reuse or replacement")
	}
	return nil
}

// Keep the existing lock files and lock ordering, but bound contention by the
// caller's context. Cancellation never removes locks or startup intent.
func launchLock(ctx context.Context, f *os.File) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := devFlock(f, devLockExclusive|devLockNonBlock)
		if err == nil {
			if err := ctx.Err(); err != nil {
				_ = devFlock(f, devLockUnlock)
				return err
			}
			return nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN && err != syscall.EINTR {
			return err
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
