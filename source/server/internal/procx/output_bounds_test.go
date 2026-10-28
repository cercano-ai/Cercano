package procx

import (
	"context"
	"testing"
	"time"
)

func TestOutputCaptureBound(t *testing.T) {
	r, e := Run(context.Background(), Options{Args: []string{"sh", "-c", "printf 123456789; printf abcdefghi >&2"}, MaxOutputBytes: 4, Timeout: time.Second})
	if e != nil || r.ExitCode != 0 || string(r.Stdout) != "1234" || string(r.Stderr) != "abcd" || !r.OutputTruncated {
		t.Fatalf("%+v %v", r, e)
	}
	r, e = Run(context.Background(), Options{Args: []string{"sh", "-c", "printf 1234"}, MaxOutputBytes: 4, Timeout: time.Second})
	if e != nil || r.OutputTruncated {
		t.Fatalf("%+v %v", r, e)
	}
}
