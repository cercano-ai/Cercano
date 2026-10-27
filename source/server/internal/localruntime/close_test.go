package localruntime

import (
	"context"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type closeProvider struct {
	Provider
	entered, canceled, release chan struct{}
	stops                      int
}

func (p *closeProvider) Name() string { return "fixture" }
func (p *closeProvider) Start(ctx context.Context, _ StartRequest, _ LogSink) (*InstanceRecord, error) {
	close(p.entered)
	<-ctx.Done()
	close(p.canceled)
	<-p.release
	// A provider may finish publishing an instance while handling cancellation.
	return &InstanceRecord{ID: "owned", Runtime: p.Name(), State: InstanceRunning}, nil
}
func (p *closeProvider) Stop(context.Context, string) error { p.stops++; return nil }
func awaitCloseSignal(t *testing.T, c <-chan struct{}) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(5 * time.Second):
		t.Fatal("lifecycle barrier timed out")
	}
}
func TestCloseJoinsWarmAndFencesLateWork(t *testing.T) {
	m := NewManager()
	p := &closeProvider{entered: make(chan struct{}), canceled: make(chan struct{}), release: make(chan struct{})}
	m.RegisterProvider(p)
	sibling := NewManager()
	defer sibling.Close()
	sibling.UpdateInstance(InstanceRecord{ID: "sibling", Runtime: "fixture", State: InstanceStopped})
	go m.Start(context.Background(), StartRequest{Runtime: "fixture"})
	awaitCloseSignal(t, p.entered)
	done := make(chan struct{})
	go func() { _ = m.Close(); close(done) }()
	awaitCloseSignal(t, p.canceled)
	select {
	case <-done:
		t.Fatal("Close returned before admitted warm joined")
	default:
	}
	close(p.release)
	awaitCloseSignal(t, done)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = m.Close() }()
	}
	wg.Wait()
	if p.stops != 1 {
		t.Fatalf("stops=%d", p.stops)
	}
	if _, err := m.Start(context.Background(), StartRequest{Runtime: "fixture"}); err == nil {
		t.Fatal("late warm admitted")
	}
	if _, err := m.DownloadModel(context.Background(), DownloadRequest{Runtime: "fixture", ModelID: "late"}); err == nil {
		t.Fatal("late resume admitted")
	}
	if _, finish, err := sibling.admitWork(context.Background()); err != nil {
		t.Fatal("sibling closed", err)
	} else {
		finish()
	}
}

type closeTransport func(*http.Request) (*http.Response, error)

func (f closeTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestCloseJoinsDownload(t *testing.T) {
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	m := NewManager()
	m.httpClient = &http.Client{Transport: closeTransport(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		close(canceled)
		<-release
		return nil, r.Context().Err()
	})}
	m.EnrollDownload(ModelRecord{ID: "download", Runtime: "fixture", Path: filepath.Join(t.TempDir(), "model"), DownloadURL: "https://fixture.invalid/model", DownloadState: DownloadNotStarted})
	if _, err := m.DownloadModel(context.Background(), DownloadRequest{Runtime: "fixture", ModelID: "download"}); err != nil {
		t.Fatal(err)
	}
	awaitCloseSignal(t, entered)
	done := make(chan struct{})
	go func() { _ = m.Close(); close(done) }()
	awaitCloseSignal(t, canceled)
	select {
	case <-done:
		t.Fatal("Close returned before download joined")
	default:
	}
	close(release)
	awaitCloseSignal(t, done)
	m.mu.RLock()
	jobs := len(m.downloadJobs)
	m.mu.RUnlock()
	if jobs != 0 {
		t.Fatalf("download jobs=%d", jobs)
	}
}
