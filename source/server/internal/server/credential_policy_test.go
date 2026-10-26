package server

import (
	"context"
	"os"
	"testing"

	"cercano/source/server/pkg/proto"
)

func TestGetCredentialPolicy(t *testing.T) {
	for _, value := range []string{"absent", "0", "1", "", "invalid"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("CERCANO_DISABLE_CREDENTIALS", value)
			if value == "absent" {
				os.Unsetenv("CERCANO_DISABLE_CREDENTIALS")
			}
			// Zero server proves this handler needs no config, runtime, or secret store.
			got, err := (&Server{}).GetCredentialPolicy(context.Background(), &proto.GetCredentialPolicyRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if want := value != "absent" && value != "0"; got.GetCredentialsDisabled() != want {
				t.Fatalf("disabled=%v want %v", got.GetCredentialsDisabled(), want)
			}
		})
	}
}
