//go:build !darwin || !cgo

package brewrestart

import (
	"context"
	"fmt"
)

func VerifyPreparedWritersAbsent(context.Context, string, string) error {
	return fmt.Errorf("prepared writer containment requires Darwin kernel process attribution")
}
