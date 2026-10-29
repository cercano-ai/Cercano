//go:build realcloudacceptance

package cloudfactory

import (
	"cercano/source/server/pkg/config"
	"errors"
)

func acceptanceProfile(p config.CloudProfile) error {
	if p.Name != "openai-responses" || p.Model != "gpt-6-astra" || p.Flavor != FlavorResponses || p.Route != "chatgpt" {
		return errors.New("real-cloud acceptance refuses alternate profile, model or route")
	}
	return nil
}
