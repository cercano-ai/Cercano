package conversation

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCoexistenceWindowsURIProbe(t *testing.T) {
	u, err := coexistenceFileURL(`C:\Users\Jane Doe\store#1?.db`, true)
	mustCoexist(t, err)
	got := u.String()
	t.Logf("constructed URI: %s", got)
	want := "file:///C:/Users/Jane%20Doe/store%231%3F.db"
	if got != want {
		t.Fatalf("URI = %q, want %q", got, want)
	}
}

func TestCoexistenceFileURL(t *testing.T) {
	for _, tt := range []struct {
		name, path, want string
		windows          bool
	}{
		{"drive", `C:\data\store.db`, "file:///C:/data/store.db", true},
		{"forward slashes", "d:/Jane Doe/store.db", "file:///d:/Jane%20Doe/store.db", true},
		{"reserved", `C:\data\a%23#?&+é.db`, "file:///C:/data/a%2523%23%3F&+%C3%A9.db", true},
		{"posix", `/tmp/a%23#?&+é.db`, "file:///tmp/a%2523%23%3F&+%C3%A9.db", false},
		{"posix backslash", `/tmp/a\b.db`, "file:///tmp/a%5Cb.db", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			u, err := coexistenceFileURL(tt.path, tt.windows)
			mustCoexist(t, err)
			if got := u.String(); got != tt.want {
				t.Fatalf("URI=%q want=%q", got, tt.want)
			}
			parsed, err := url.Parse(u.String())
			mustCoexist(t, err)
			if parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
				t.Fatalf("path escaped into URI fields: %+v", parsed)
			}
		})
	}
	for _, path := range []string{`\\server\share\store.db`, "//server/share/store.db", `\\?\C:\data\store.db`, `\\.\C:\data\store.db`, `C:store.db`} {
		if _, err := coexistenceFileURL(path, true); err == nil {
			t.Errorf("accepted unsupported Windows path %q", path)
		}
	}
}

func TestCoexistenceURIOptions(t *testing.T) {
	for _, mode := range []string{"ro", "rw"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "a %23#&+é.db")
			uri, err := coexistenceURI(path, mode)
			mustCoexist(t, err)
			t.Logf("constructed URI: %s", uri)
			u, err := url.Parse(uri)
			mustCoexist(t, err)
			want := url.Values{"mode": {mode}, "_pragma": {"busy_timeout(5000)"}}
			if mode == "rw" {
				want["_pragma"] = append(want["_pragma"], "foreign_keys(1)")
				want["_txlock"] = []string{"immediate"}
			}
			if !reflect.DeepEqual(u.Query(), want) {
				t.Fatalf("query=%v want=%v", u.Query(), want)
			}
			if u.Host != "" || u.Fragment != "" || !strings.HasSuffix(u.Path, "/a %23#&+é.db") {
				t.Fatalf("incorrect URI: %+v", u)
			}
		})
	}
	for _, path := range []string{"", ":memory:"} {
		if _, err := coexistenceURI(path, "rw"); err == nil {
			t.Errorf("accepted %q", path)
		}
	}
}

func TestCoexistenceURIReservedFilenameIsolation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store %23#&+é.db")
	mustCoexist(t, PrepareCoexistence(ctx, path))
	mustCoexist(t, PreflightCoexistence(ctx, path, CurrentCoexistenceDeclaration()))
	s, err := OpenCoexistence(ctx, path, CurrentCoexistenceDeclaration())
	mustCoexist(t, err)
	mustCoexist(t, s.Close())
	ro, err := openCoexistenceDB(path, "ro")
	mustCoexist(t, err)
	defer ro.Close()
	if _, err := ro.Exec("CREATE TABLE forbidden(id INTEGER)"); err == nil {
		t.Fatal("read-only URI permitted writes")
	}
	var busy int
	mustCoexist(t, ro.QueryRow("PRAGMA busy_timeout").Scan(&busy))
	if busy != 5000 {
		t.Fatalf("busy_timeout=%d", busy)
	}
	missing := filepath.Join(t.TempDir(), "missing #%.db")
	db, err := openCoexistenceDB(missing, "ro")
	mustCoexist(t, err)
	defer db.Close()
	if err := db.Ping(); err == nil {
		t.Fatal("read-only URI opened a missing database")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("read-only open created missing file: %v", err)
	}
}
