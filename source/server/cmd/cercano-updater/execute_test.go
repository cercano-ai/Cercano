package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cercano/source/server/internal/updatecoord/exclusion"
	"cercano/source/server/internal/updatecoord/nativeprocess"
	"cercano/source/server/pkg/installcontext"
	"cercano/source/server/pkg/installcontexttest"
	"cercano/source/server/pkg/proto"
	"google.golang.org/grpc"
)

type inertDrain struct{ proto.UnimplementedAgentServer }

func (*inertDrain) GetAgentHealth(context.Context, *proto.GetAgentHealthRequest) (*proto.GetAgentHealthResponse, error) {
	return &proto.GetAgentHealthResponse{CredentialsDisabled: true}, nil
}
func (*inertDrain) DrainForUpdate(r *proto.DrainForUpdateRequest, s proto.Agent_DrainForUpdateServer) error {
	fmt.Println("drain")
	for _, phase := range []proto.UpdateDrainState_Phase{proto.UpdateDrainState_SEALED, proto.UpdateDrainState_DRAINING, proto.UpdateDrainState_IDLE} {
		if err := s.Send(&proto.UpdateDrainState{OperationToken: r.OperationToken, Phase: phase}); err != nil {
			return err
		}
	}
	<-s.Context().Done()
	fmt.Println("released")
	return s.Context().Err()
}
func TestMain(m *testing.M) {
	switch os.Getenv("CERCANO_INERT_UPDATER_PARTICIPANT") {
	case "agent":
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			panic(err)
		}
		server := grpc.NewServer()
		proto.RegisterAgentServer(server, &inertDrain{})
		fmt.Println(l.Addr().String())
		_ = server.Serve(l)
		return
	case "client":
		fmt.Println("client")
		for {
			time.Sleep(time.Hour)
		}
	}
	os.Exit(m.Run())
}
func startInert(t *testing.T, f *installcontexttest.Install, role string) (nativeprocess.Identity, <-chan string) {
	t.Helper()
	path := f.AgentExe
	if role == "client" {
		path = f.ClientExe
	}
	cmd := exec.Command(path)
	cmd.Env = append(f.ChildEnv(), "CERCANO_INERT_UPDATER_PARTICIPANT="+role)
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }) // test-owned inert cleanup only
	lines := make(chan string, 8)
	go func() {
		defer close(lines)
		s := bufio.NewScanner(pipe)
		for s.Scan() {
			lines <- s.Text()
		}
	}()
	id, err := nativeprocess.Inspect(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	return id, lines
}
func nextLine(t *testing.T, lines <-chan string) string {
	t.Helper()
	select {
	case s := <-lines:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("inert participant timeout")
		return ""
	}
}
func inputFile(t *testing.T, in operationInput) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "operation.json")
	data, _ := json.Marshal(in)
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func sampleInput() operationInput {
	return operationInput{InstallRoot: "/tmp/install", InstallID: strings.Repeat("a", 32), StateRoot: "/tmp/state", OperationToken: strings.Repeat("b", 32), AgentEndpoint: "127.0.0.1:12345", StatusFile: "status.json", Participants: []participant{{Role: "agent", Identity: nativeprocess.Identity{PID: 1, UID: uint32(os.Getuid()), Executable: "/tmp/install/versions/old/bin/cercano", StartSeconds: 1}}}}
}
func TestStrictOperationInput(t *testing.T) {
	good, _ := json.Marshal(sampleInput())
	if _, err := parseInput(good); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"unknown":            []byte(strings.Replace(string(good), "{", "{\"extra\":1,", 1)),
		"duplicate":          []byte(strings.Replace(string(good), "{", "{\"install_id\":\"x\",", 1)),
		"duplicate identity": []byte(strings.Replace(string(good), "\"PID\":1", "\"PID\":1,\"PID\":1", 1)),
		"wrong case":         []byte(strings.Replace(string(good), "\"PID\"", "\"pid\"", 1)),
		"missing":            []byte(strings.Replace(string(good), "\"WindowsSID\":\"\",", "", 1)),
		"null":               []byte(strings.Replace(string(good), "\"role\":\"agent\"", "\"role\":null", 1)),
		"trailing":           append(append([]byte{}, good...), []byte(" {}")...),
		"oversize":           bytesRepeat(' ', maxInput+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseInput(data); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	for _, mutate := range []func(*operationInput){func(in *operationInput) { in.Participants = nil }, func(in *operationInput) { in.Participants = append(in.Participants, in.Participants[0]) }, func(in *operationInput) { in.Participants = make([]participant, maxParticipants+1) }, func(in *operationInput) { in.StatusFile = "../status.json" }, func(in *operationInput) { in.AgentEndpoint = "0.0.0.0:12345" }, func(in *operationInput) { in.OperationToken = strings.Repeat("A", 32) }} {
		in := sampleInput()
		mutate(&in)
		data, _ := json.Marshal(in)
		if _, err := parseInput(data); err == nil {
			t.Fatal("accepted invalid schema")
		}
	}
	path := inputFile(t, sampleInput())
	if _, err := readInput(path); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(path, 0644)
	if _, err := readInput(path); err == nil {
		t.Fatal("accepted public input")
	}
}
func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func TestConnectedOwnerEvidenceOnly(t *testing.T) {
	t.Setenv("CERCANO_DISABLE_CREDENTIALS", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f := installcontexttest.Build(t, exe, exe)
	// Corrupt SQL rather than consulting or repairing it: boot selection and
	// exclusion are the only local update authority used in this slice.
	bootDir, err := installcontext.BootDirectory(f.StateRoot, f.InstallID)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"state.db", "state.db-wal", "state.db-shm"} {
		if err := os.Remove(filepath.Join(bootDir, name)); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bootDir, "state.db"), []byte("deliberately invalid SQL"), 0600); err != nil {
		t.Fatal(err)
	}

	agent, agentLines := startInert(t, f, "agent")
	endpoint := nextLine(t, agentLines)
	client, clientLines := startInert(t, f, "client")
	if nextLine(t, clientLines) != "client" {
		t.Fatal("client not ready")
	}
	in := sampleInput()
	in.InstallRoot = f.Root
	in.InstallID = f.InstallID
	in.StateRoot = f.StateRoot
	in.AgentEndpoint = endpoint
	in.Participants = []participant{{"agent", agent}, {"client", client}}
	// All participants (including disconnected clients) must be inspected before
	// the first RPC. A changed/unknown client refuses without asking agent drain.
	for _, kind := range []string{"changed", "unknown", "wrong role", "wrong listener"} {
		t.Run(kind, func(t *testing.T) {
			bad := in
			bad.Participants = append([]participant{}, in.Participants...)
			switch kind {
			case "changed":
				bad.Participants[1].Identity.StartSeconds++
			case "unknown":
				bad.Participants[1].Identity.PID = 2147483647
			case "wrong role":
				bad.Participants[1].Identity.Executable = f.AgentExe
			case "wrong listener":
				bad.AgentEndpoint = "127.0.0.1:1"
			}
			path := inputFile(t, bad)
			if err := execute(context.Background(), path); err == nil {
				t.Fatal("accepted missing evidence")
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), "status.json")); !os.IsNotExist(err) {
				t.Fatal("wrote readiness on refusal")
			}
			select {
			case line := <-agentLines:
				t.Fatalf("RPC before evidence: %s", line)
			default:
			}
		})
	}
	path := inputFile(t, in)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- execute(ctx, path) }()
	if nextLine(t, agentLines) != "drain" {
		t.Fatal("missing drain")
	}
	statusPath := filepath.Join(filepath.Dir(path), "status.json")
	deadline := time.Now().Add(3 * time.Second)
	for {
		data, err := os.ReadFile(statusPath)
		if err == nil && strings.Contains(string(data), "owner_evidence_sealed_incomplete") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no sealed status")
		}
		time.Sleep(10 * time.Millisecond)
	}
	dir, err := installcontext.BootDirectory(f.StateRoot, f.InstallID)
	if err != nil {
		t.Fatal(err)
	}
	blocked, stop := context.WithTimeout(context.Background(), 50*time.Millisecond)
	lease, err := exclusion.Acquire(blocked, dir, exclusion.Launch)
	stop()
	if lease != nil {
		lease.Close()
		t.Fatal("barrier not held")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	// IDLE and all processes still live never means installed or exit proof.
	select {
	case err := <-done:
		t.Fatalf("returned before cancellation: %v", err)
	default:
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, errIncomplete) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("owner cancellation stuck")
	}
	if nextLine(t, agentLines) != "released" {
		t.Fatal("seal not released")
	}
	lease, err = exclusion.Acquire(context.Background(), dir, exclusion.Update)
	if err != nil {
		t.Fatal(err)
	}
	lease.Close()

	// A deadline also releases reversible ownership without any exit request.
	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), time.Second)
	defer deadlineCancel()
	deadlinePath := inputFile(t, in)
	deadlineDone := make(chan error, 1)
	go func() { deadlineDone <- execute(deadlineCtx, deadlinePath) }()
	if nextLine(t, agentLines) != "drain" {
		t.Fatal("missing second drain")
	}
	if err := <-deadlineDone; !errors.Is(err, errIncomplete) {
		t.Fatal(err)
	}
	if nextLine(t, agentLines) != "released" {
		t.Fatal("deadline retained seal")
	}
	for _, want := range []nativeprocess.Identity{agent, client} {
		got, err := nativeprocess.Inspect(want.PID)
		if err != nil || !got.SameProcess(want) {
			t.Fatalf("participant was stopped: %v", err)
		}
	}
}
