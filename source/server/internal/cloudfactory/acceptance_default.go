//go:build !realcloudacceptance

package cloudfactory

import "cercano/source/server/pkg/config"

func acceptanceProfile(config.CloudProfile) error { return nil }
