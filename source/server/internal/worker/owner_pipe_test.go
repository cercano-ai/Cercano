package worker

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	wire "cercano/source/server/pkg/proto"
)

func TestOwnerPipeChild(t *testing.T) {
	mode := os.Getenv("CERCANO_OWNER_TEST")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	if mode == "tool" {
		time.Sleep(time.Hour)
		return
	}
	bind, err := WatchOwnerLifetime()
	if err != nil {
		t.Fatal(err)
	}
	if mode != "startup" {
		w := New()
		bind(w)
		if mode == "blocked-cleanup" {
			w.accountingMu.Lock()
		}
		if mode == "active" || mode == "stubborn" {
			ctx, release, err := w.beginAccountingTurn(context.Background(), &wire.AccountingWork{}, "test")
			if err != nil {
				t.Fatal(err)
			}
			var descendant *exec.Cmd
			if mode == "active" {
				descendant = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestOwnerPipeChild$")
				descendant.Env = append(os.Environ(), "CERCANO_OWNER_TEST=tool")
				if err := descendant.Start(); err != nil {
					t.Fatal(err)
				}
			}
			go func() {
				<-ctx.Done()
				if _, _, err := w.beginAccountingTurn(context.Background(), nil, "late"); err == nil {
					os.Exit(9)
				}
				fmt.Println("canceled-and-fenced")
				if mode == "active" {
					if err := descendant.Wait(); err == nil {
						os.Exit(8)
					}
					fmt.Println("descendant-reaped")
					release()
				}
			}()
		}
	}
	fmt.Println("ready")
	select {}
}

func TestOwnerPipeSubprocess(t *testing.T) {
	for _, mode := range []string{"idle", "startup", "active", "stubborn", "blocked-cleanup"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestOwnerPipeChild$")
			cmd.Env = append(os.Environ(), "CERCANO_OWNER_TEST="+mode)
			r, w, err := prepareOwnerPipe(cmd)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			defer w.Close()
			out, outputWriter, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			defer outputWriter.Close()
			cmd.Stdout = outputWriter
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			r.Close()
			outputWriter.Close()
			done := make(chan struct{})
			var waitErr error
			go func() { waitErr = cmd.Wait(); close(done) }()
			defer func() { cmd.Process.Kill(); <-done }()
			lines := make(chan string, 10)
			go func() {
				s := bufio.NewScanner(out)
				for s.Scan() {
					lines <- s.Text()
				}
				close(lines)
			}()
			select {
			case line := <-lines:
				if line != "ready" {
					t.Fatalf("startup: %q", line)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("startup timeout")
			}
			// Absence of gRPC traffic must not be mistaken for owner death.
			select {
			case line := <-lines:
				t.Fatalf("exited while owner alive: %q", line)
			case <-time.After(100 * time.Millisecond):
			}
			start := time.Now()
			w.Close()
			select {
			case <-done:
				if e, ok := waitErr.(*exec.ExitError); !ok || e.ExitCode() != 1 {
					t.Fatalf("exit: %v", waitErr)
				}
			case <-time.After(6 * time.Second):
				t.Fatal("owner loss did not bound exit")
			}
			canceled := false
			reaped := false
			for line := range lines {
				if line == "descendant-reaped" {
					reaped = true
				}
				if line == "canceled-and-fenced" {
					canceled = true
				}
			}
			if mode == "active" && !reaped {
				t.Fatal("descendant not reaped")
			}
			if (mode == "active" || mode == "stubborn") && !canceled {
				t.Fatal("producer not canceled/fenced")
			}
			if mode == "stubborn" && time.Since(start) < time.Second {
				t.Fatal("did not exercise cleanup budget")
			}
		})
	}
}
