package archivecheck

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"
)

func TestCompleteReleaseMembers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		members []string
		ok      bool
	}{
		{"complete", []string{"bin/cercano", "bin/cercano-cli", "bin/cercano-updater"}, true},
		{"missing updater", []string{"bin/cercano", "bin/cercano-cli"}, false},
		{"unexpected", []string{"bin/cercano", "bin/cercano-cli", "bin/cercano-updater", "bin/other"}, false},
		{"duplicate updater", []string{"bin/cercano", "bin/cercano-cli", "bin/cercano-updater", "bin/cercano-updater"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b bytes.Buffer
			z := zip.NewWriter(&b)
			for _, name := range tc.members {
				w, err := z.Create(name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = w.Write([]byte("member")); err != nil {
					t.Fatal(err)
				}
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			_, err := Check(context.Background(), Archive{ReaderAt: bytes.NewReader(b.Bytes()), Size: int64(b.Len())}, Options{Format: Zip, Layout: CompleteReleaseLayout(), Bounds: Bounds{MaxMembers: 10, MaxCompressedBytes: 4096, MaxUncompressedBytes: 4096, MaxMemberBytes: 1024}})
			if (err == nil) != tc.ok {
				t.Fatalf("err=%v want success=%v", err, tc.ok)
			}
		})
	}
}
