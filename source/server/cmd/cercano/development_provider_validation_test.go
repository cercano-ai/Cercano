package main

import (
	"cercano/source/server/internal/secrets"
	"cercano/source/server/pkg/config"
	"testing"
)

func TestDevelopmentCloudStaticValidationIndependentOfCredentials(t *testing.T) {
	for _, policy := range []string{"0", "1"} {
		t.Run(policy, func(t *testing.T) {
			t.Setenv(secrets.DisableEnv, policy)
			// Pure validation only: no store, provider, or network is constructed.
			for _, tc := range []struct {
				name  string
				cfg   config.Config
				valid bool
			}{
				{"empty", config.Config{}, true},
				{"supported", config.Config{ActiveCloudProfile: "p", CloudProfiles: []config.CloudProfile{{Name: "p", Flavor: "messages"}}}, true},
				{"unknown flavor", config.Config{CloudProfiles: []config.CloudProfile{{Name: "p", Flavor: "bogus"}}}, false},
				{"unnamed", config.Config{CloudProfiles: []config.CloudProfile{{Flavor: "messages"}}}, false},
				{"duplicate", config.Config{CloudProfiles: []config.CloudProfile{{Name: "p", Flavor: "messages"}, {Name: "p", Flavor: "responses"}}}, false},
				{"missing active", config.Config{ActiveCloudProfile: "missing"}, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if err := validateDevelopmentCloudProfiles(tc.cfg); (err == nil) != tc.valid {
						t.Fatalf("valid=%v err=%v", tc.valid, err)
					}
				})
			}
		})
	}
}
