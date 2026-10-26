package server

import (
	"context"

	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/proto"
)

// GetCredentialPolicy reports only the fail-closed credential boundary. It does
// not open credentials, load configuration, or imply readiness of any runtime.
func (s *Server) GetCredentialPolicy(context.Context, *proto.GetCredentialPolicyRequest) (*proto.GetCredentialPolicyResponse, error) {
	return &proto.GetCredentialPolicyResponse{CredentialsDisabled: secrets.CheckAllowed() != nil}, nil
}
