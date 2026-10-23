//go:build darwin || linux

package main

import (
	"golang.org/x/sys/unix"
	"os"
)

const protectedReadFlags = os.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK
