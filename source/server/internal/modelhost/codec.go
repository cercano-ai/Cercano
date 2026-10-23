// Package modelhost implements the shared local-model host: the gRPC protocol
// (see source/proto/modelhost.proto) and its error codec.
package modelhost

import (
	"errors"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"cercano/source/server/internal/localruntime"
)

// APIVersion is the model-host wire protocol version, exchanged in Hello.
// Bump it whenever a method's payload shape changes.
const APIVersion = 1

// errorDomain scopes the ErrorInfo detail carrying a sentinel's name.
const errorDomain = "cercano.modelhost"

// sentinels maps the exported localruntime.Err* sentinels to their wire name
// and gRPC code. Add new exported sentinels here.
var sentinels = []struct {
	name string
	err  error
	code codes.Code
}{
	{"ErrModelNotFound", localruntime.ErrModelNotFound, codes.NotFound},
	{"ErrModelNotPresent", localruntime.ErrModelNotPresent, codes.FailedPrecondition},
}

// sentinelName returns the wire name of a wrapped localruntime sentinel.
func sentinelName(err error) (string, bool) {
	for _, s := range sentinels {
		if errors.Is(err, s.err) {
			return s.name, true
		}
	}
	return "", false
}

// encodeError converts err into a gRPC status error. A wrapped localruntime
// sentinel becomes NotFound or FailedPrecondition with the sentinel's name in
// an ErrorInfo detail; the full message is preserved. Other errors become
// Unknown (or keep their existing status).
func encodeError(err error) error {
	if err == nil {
		return nil
	}
	for _, s := range sentinels {
		if !errors.Is(err, s.err) {
			continue
		}
		st := status.New(s.code, err.Error())
		if withInfo, derr := st.WithDetails(&errdetails.ErrorInfo{Reason: s.name, Domain: errorDomain}); derr == nil {
			st = withInfo
		}
		return st.Err()
	}
	if _, ok := status.FromError(err); ok {
		return err
	}
	return status.Error(codes.Unknown, err.Error())
}

// decodeError restores the localruntime sentinel encoded by encodeError so
// errors.Is works on the client. The returned error keeps the server's message.
func decodeError(err error) error {
	if err == nil {
		return nil
	}
	st, ok := status.FromError(err)
	if !ok {
		return err
	}
	for _, d := range st.Details() {
		info, ok := d.(*errdetails.ErrorInfo)
		if !ok || info.GetDomain() != errorDomain {
			continue
		}
		for _, s := range sentinels {
			if s.name == info.GetReason() {
				return &remoteError{msg: st.Message(), sentinel: s.err}
			}
		}
	}
	// No recognised sentinel: keep the status error so status.Code still works
	// (Unavailable, Canceled, DeadlineExceeded).
	return err
}

type remoteError struct {
	msg      string
	sentinel error
}

func (e *remoteError) Error() string { return e.msg }
func (e *remoteError) Unwrap() error { return e.sentinel }
