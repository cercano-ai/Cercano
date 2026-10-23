package brewrestart

import (
	"reflect"
	"testing"
)

func TestParseProcStatPPID(t *testing.T) {
	ppid, err := parseProcStatPPID([]byte("4242 (cer) (cano x) S 17 4242 4242 0 -1"))
	if err != nil || ppid != 17 {
		t.Fatalf("ppid = %d, %v; want 17", ppid, err)
	}
	for _, bad := range []string{"", "4242 cercano S 17", "4242 (cercano) S", "4242 (cercano) S x"} {
		if _, err := parseProcStatPPID([]byte(bad)); err == nil {
			t.Errorf("parseProcStatPPID(%q) accepted", bad)
		}
	}
}

func TestSplitNULAndEnvHome(t *testing.T) {
	if got := splitNUL([]byte("cercano\x00--mcp\x00\x00")); !reflect.DeepEqual(got, []string{"cercano", "--mcp", ""}) {
		t.Errorf("splitNUL = %q", got)
	}
	if got := splitNUL(nil); got != nil {
		t.Errorf("splitNUL(nil) = %q", got)
	}
	if got := envHome([]string{"PATH=/bin", "HOME=/Users/a", "HOMEDIR=/x"}); got != "/Users/a" {
		t.Errorf("envHome = %q", got)
	}
	if got := envHome([]string{"HOMEDIR=/x"}); got != "" {
		t.Errorf("envHome without HOME = %q", got)
	}
}
