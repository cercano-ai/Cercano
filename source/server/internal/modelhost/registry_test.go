package modelhost

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestWriteRecordIsPrivateAndAtomic(t *testing.T) {
	dir := t.TempDir()
	rec := Record{PID: 1, Addr: "127.0.0.1:1", Token: "t", APIVersion: 1, Version: "v", StartedAt: time.Unix(5, 0).UTC()}
	if err := WriteRecord(dir, rec); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(dir, "modelhost.json"))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, want 0600", fi.Mode().Perm())
		}
	}

	var wg sync.WaitGroup
	done := make(chan struct{})
	bad := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
			}
			if _, err := ReadRecord(dir); err != nil {
				select {
				case bad <- err:
				default:
				}
				return
			}
		}
	}()
	for i := 0; i < 200; i++ {
		rec.PID = i + 1
		if err := WriteRecord(dir, rec); err != nil {
			t.Fatal(err)
		}
	}
	close(done)
	wg.Wait()
	select {
	case err := <-bad:
		t.Fatalf("concurrent read failed: %v", err)
	default:
	}
	got, err := ReadRecord(dir)
	if err != nil || got.PID != 200 || got.Addr != rec.Addr || !got.StartedAt.Equal(rec.StartedAt) {
		t.Fatalf("got %+v, %v", got, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("leftover temp files: %v", entries)
	}
}

func TestRemoveRecordIgnoresOtherPID(t *testing.T) {
	dir := t.TempDir()
	if err := WriteRecord(dir, Record{PID: 10}); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRecord(dir, 11); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRecord(dir); err != nil {
		t.Fatalf("record should survive: %v", err)
	}
	if err := RemoveRecord(dir, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadRecord(dir); !os.IsNotExist(err) {
		t.Fatalf("want not exist, got %v", err)
	}
	if err := RemoveRecord(dir, 10); err != nil {
		t.Fatalf("removing absent record: %v", err)
	}
}
