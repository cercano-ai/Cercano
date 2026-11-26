package worker

import (
	"context"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

const ownerPipeEnv = "CERCANO_WORKER_OWNER_PIPE"

// WatchOwnerLifetime must run before startup hooks or accepting requests. The
// returned binder publishes the server to the watcher before serving. Manual
// workers without an inherited pipe retain their existing standalone behavior.
// Transport loss is deliberately unrelated to this signal.
func WatchOwnerLifetime() (func(*WorkerServer), error) {
	value := os.Getenv(ownerPipeEnv)
	if value == "" {
		return func(*WorkerServer) {}, nil
	}
	_ = os.Unsetenv(ownerPipeEnv) // do not advertise our descriptor to descendants
	r, err := inheritedOwnerPipe(value)
	if err != nil {
		return nil, err
	}
	return watchOwnerPipe(r, os.Exit), nil
}

func watchOwnerPipe(r io.ReadCloser, exit func(int)) func(*WorkerServer) {
	var mu sync.Mutex
	var server *WorkerServer
	lost := false
	go func() {
		// No payload protocol: any terminal read error is ownership loss.
		_, _ = io.Copy(io.Discard, r)
		_ = r.Close()
		// Independent of cleanup locks and cancellation callbacks: even a stuck
		// shutdown implementation must not strand an ownerless worker.
		deadline := time.AfterFunc(3*time.Second, func() { exit(1) })
		defer deadline.Stop()
		mu.Lock()
		lost = true
		w := server
		if w != nil {
			w.startAccountingClose()
		} // synchronous admission fence/cancel
		mu.Unlock()
		// Never wait indefinitely for a provider, collector, or lost acknowledgement.
		// Owner loss cannot establish successful host persistence, even if local
		// cleanup succeeds, so this process always exits unsuccessfully.
		if w != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			err := w.CloseAccounting(ctx)
			cancel()
			if err != nil {
				log.Printf("[worker] owner lost; cleanup incomplete: %v", err)
			}
		}
		exit(1)
	}()
	return func(w *WorkerServer) {
		mu.Lock()
		defer mu.Unlock()
		server = w
		if lost {
			w.startAccountingClose()
		}
	}
}
