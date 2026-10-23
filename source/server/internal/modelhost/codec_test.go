package modelhost

import (
	"errors"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cercano/source/server/internal/localruntime"
)

func TestErrorSentinelsRoundTrip(t *testing.T) {
	sentinels := []error{
		localruntime.ErrModelNotFound,
		localruntime.ErrModelNotPresent,
	}
	for _, s := range sentinels {
		wire := encodeError(fmt.Errorf("wrap: %w", s))
		if st, ok := status.FromError(wire); !ok || st.Code() == codes.OK || st.Code() == codes.Unknown {
			t.Errorf("%v: encoded to unexpected status %v", s, wire)
		}
		got := decodeError(wire)
		if !errors.Is(got, s) {
			t.Errorf("errors.Is(decode(encode(%v)), sentinel) = false; got %v", s, got)
		}
	}
}

func TestErrorCodecPassthrough(t *testing.T) {
	if encodeError(nil) != nil || decodeError(nil) != nil {
		t.Fatal("nil must stay nil")
	}
	plain := errors.New("boom")
	got := decodeError(encodeError(plain))
	if got == nil || errors.Is(got, localruntime.ErrModelNotFound) || errors.Is(got, localruntime.ErrModelNotPresent) {
		t.Fatalf("plain error mis-decoded: %v", got)
	}
}

func TestDecodeErrorKeepsStatusCodes(t *testing.T) {
	for _, c := range []codes.Code{codes.Unavailable, codes.Canceled, codes.DeadlineExceeded} {
		if got := status.Code(decodeError(status.Error(c, "x"))); got != c {
			t.Errorf("status code %v became %v", c, got)
		}
	}
	plain := errors.New("plain")
	if got := decodeError(plain); got != plain {
		t.Errorf("non-status error changed: %v", got)
	}
	if got := status.Code(decodeError(encodeError(errors.New("boom")))); got != codes.Unknown {
		t.Errorf("plain error code = %v, want Unknown", got)
	}
}
