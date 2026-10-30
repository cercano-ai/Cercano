package main

import (
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"cercano/source/server/pkg/config"
)

// withStdin swaps os.Stdin for the duration of a test.
func withStdin(t *testing.T, f *os.File) {
	t.Helper()
	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = orig })
}

// TestACPInitializeHandshake drives runACPMode's request loop over a pipe pair
// and checks the negotiated protocol version comes back on stdout.
func TestACPInitializeHandshake(t *testing.T) {
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	origOut := os.Stdout
	os.Stdout = outW
	t.Cleanup(func() { os.Stdout = origOut })
	withStdin(t, inR)

	go func() {
		_, _ = inW.WriteString(`{"jsonrpc":"2.0","id":7,"method":"initialize","params":{"protocolVersion":1}}` + "\n")
		_, _ = inW.WriteString(`{"jsonrpc":"2.0","id":8,"method":"session/prompt"}` + "\n")
		inW.Close()
	}()

	done := make(chan struct{})
	go func() {
		runACPMode(config.Defaults(), nil)
		outW.Close()
		close(done)
	}()

	dec := json.NewDecoder(outR)

	var initResp acpResponse
	if err := dec.Decode(&initResp); err != nil {
		t.Fatalf("decode initialize response: %v", err)
	}
	if initResp.Error != nil {
		t.Fatalf("initialize returned error: %+v", initResp.Error)
	}
	result, ok := initResp.Result.(map[string]any)
	if !ok {
		t.Fatalf("initialize result shape: %T", initResp.Result)
	}
	if v, _ := result["protocolVersion"].(float64); int(v) != acpProtocolVersion {
		t.Fatalf("protocolVersion = %v, want %d", result["protocolVersion"], acpProtocolVersion)
	}

	// Malformed prompts must fail validation rather than hang the editor.
	var promptResp acpResponse
	if err := dec.Decode(&promptResp); err != nil {
		t.Fatalf("decode prompt response: %v", err)
	}
	if promptResp.Error == nil || promptResp.Error.Code != -32602 {
		t.Fatalf("session/prompt error = %+v, want code -32602", promptResp.Error)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("runACPMode did not exit after client hung up")
	}
}

// TestACPNotificationGetsNoReply pins the JSON-RPC rule that id-less requests
// are notifications and must not be answered.
func TestACPNotificationGetsNoReply(t *testing.T) {
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	origOut := os.Stdout
	os.Stdout = outW
	t.Cleanup(func() { os.Stdout = origOut })
	withStdin(t, inR)

	go func() {
		_, _ = inW.WriteString(`{"jsonrpc":"2.0","method":"session/cancel"}` + "\n")
		inW.Close()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		runACPMode(config.Defaults(), nil)
		outW.Close()
	}()

	got, err := io.ReadAll(outR)
	<-done
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("notification produced a reply: %q", got)
	}
}

func TestACPOutputCloseInterruptsBlockedPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	output, err := acpOutput(w)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	done := make(chan error, 1)
	go func() { _, err := output.Write(make([]byte, 8<<20)); done <- err }()
	// A byte proves the writer entered the pipe; leave the remainder unread.
	if _, err := r.Read(make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() { closed <- output.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("closing ACP output blocked")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("blocked write unexpectedly completed")
		}
	case <-time.After(time.Second):
		t.Fatal("output Close did not interrupt Write")
	}
}
