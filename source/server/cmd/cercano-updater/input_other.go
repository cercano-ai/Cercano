//go:build !darwin && !linux

package main

import "os"

const protectedReadFlags = os.O_RDONLY
