//go:build !darwin || !cgo

package brewrestart

import (
	"context"
	"fmt"
)

func VerifyHostAbsent(context.Context, string) error {
	return fmt.Errorf("kernel-verified host recovery unsupported on this platform")
}
