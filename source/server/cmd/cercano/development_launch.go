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
	Endpoint     string
	TokenFile    string
	Identity     *pb.LifecycleIdentity
	Conversation string
	Epoch        string
	Generation   string
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
	owner, ok := st.Sys().(*syscall.Stat_t)
	if !st.IsDir() || st.Mode().Perm()&0077 != 0 || !ok || owner.Uid != uint32(os.Getuid()) {
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
	owner, ok := st.Sys().(*syscall.Stat_t)
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || !ok || owner.Uid != uint32(os.Getuid()) {
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
func runDevelopmentLaunch(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("development-launch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
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
	resolved, e := filepath.EvalSymlinks(*generation)
	if e != nil {
		return e
	}
	if !filepath.IsAbs(*generation) || resolved != *generation {
		return fmt.Errorf("canonical absolute generation required")
	}
	contract := conversation.DevelopmentCoexistenceDeclaration()
	store := filepath.Join(*state, "conversations.db")
	for _, p := range []string{store, filepath.Join(*state, "config.yaml")} {
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
	b, e := os.ReadFile(filepath.Join(*generation, "manifest.json"))
	if e != nil {
		return e
	}
	if e = json.Unmarshal(b, &manifest); e != nil {
		return e
	}
	binary := filepath.Join(*generation, "cercano")
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
	hosts := filepath.Join(*state, "hosts")
	selections := filepath.Join(*state, "selections")
	for _, p := range []string{hosts, selections} {
		if e = os.Mkdir(p, 0700); e != nil && !os.IsExist(e) {
			return e
		}
		if e = launchPrivateDir(p); e != nil {
			return e
		}
	}
	key := sha256.Sum256([]byte(*generation))
	base := filepath.Join(hosts, hex.EncodeToString(key[:]))
	lock, e := os.OpenFile(base+".lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); e != nil {
		return e
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	record := base + ".json"
	var s launchSelection
	e = launchRead(record, &s)
	if os.IsNotExist(e) {
		host, err := launchID()
		if err != nil {
			return err
		}
		// Never delete this intent automatically, even if spawning or readiness fails.
		if e = launchExclusive(base+".starting", map[string]string{"host": host, "generation": *generation}); e != nil {
			return fmt.Errorf("prior start intent exists; explicit verified retirement required: %w", e)
		}
		token := base + ".token"
		if e = devhostauth.Create(token); e != nil {
			return e
		}
		log, e := os.OpenFile(base+".log", os.O_CREATE|os.O_APPEND|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
		if e != nil {
			return e
		}
		cmd := exec.Command(binary, "development-host", "--state", *state, "--generation", *generation, "--host", host, "--token-file", token, "--record", record, "--runtime-root", *runtime, "--listen", "127.0.0.1:0")
		cmd.Env = append(os.Environ(), "CERCANO_DISABLE_CREDENTIALS=1")
		cmd.Stdout = log
		cmd.Stderr = log
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
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
	if expected.Host == "" || !proto.Equal(s.Identity, expected) || s.Conversation != "" || s.Epoch != "" || s.TokenFile != base+".token" {
		return fmt.Errorf("host record identity mismatch")
	}
	ready, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	controls, e := launchControls(ready, s)
	if e != nil {
		return e
	}
	defer controls.Close()
	check, e := controls.LifecycleList(ready, &pb.LifecycleControlRequest{Target: expected, Management: true})
	if e != nil {
		return fmt.Errorf("recorded host unreachable or unavailable; refusing replacement: %w", e)
	}
	if !proto.Equal(check.Identity, expected) {
		return fmt.Errorf("authenticated readiness identity mismatch")
	}
	id, e := launchID()
	if e != nil {
		return e
	}
	acquired, e := controls.LifecycleAcquire(ready, &pb.LifecycleControlRequest{Target: expected, Owner: &pb.LifecycleOwner{Conversation: id}})
	if e != nil {
		return e
	}
	if !proto.Equal(acquired.Identity, expected) || acquired.GetOwner().GetConversation() != id || acquired.GetOwner().GetHost() != expected.Host || acquired.GetOwner().GetEpoch() == "" {
		return fmt.Errorf("acquire identity mismatch")
	}
	s.Conversation = id
	s.Epoch = acquired.Owner.Epoch
	s.Generation = *generation
	selection := filepath.Join(selections, id+".json")
	if e = launchExclusive(selection, s); e != nil {
		return e
	}
	_, e = fmt.Fprintln(out, selection)
	return e
}
