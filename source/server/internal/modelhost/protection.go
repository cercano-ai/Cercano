package modelhost

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// ErrCoexistenceProtected reports an operation forbidden during coexistence.
var ErrCoexistenceProtected = status.Error(codes.FailedPrecondition, "shared model runtime is protected for coexistence")

const protectionHeader = "x-cercano-modelhost-coexistence-protection"

func (s *Server) protected(ctx context.Context) bool {
	if s.opts.CoexistenceProtection {
		return true
	}
	md, _ := metadata.FromIncomingContext(ctx)
	for _, v := range md.Get(protectionHeader) {
		if v == "true" {
			return true
		}
	}
	return false
}

func disruptiveMethod(method string) bool {
	switch method {
	case "Restart", "DownloadModel", "EnsureModelsPresent", "CancelDownload", "DeleteModel", "EnrollDownload":
		return true
	}
	return false
}
