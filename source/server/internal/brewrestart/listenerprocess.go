package brewrestart

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
)

// ListenerProcess and ProcessExited are implemented per platform:
//   - macOS with cgo: libproc (the same calls the restart coordinator uses),
//   - Linux: /proc,
//   - elsewhere: errors.ErrUnsupported.
//
// ListenerProcess returns the same-user process holding a TCP listener that
// accepts connections to endpoint (bound to its address or a wildcard). found
// is false with a nil error when there is none. ProcessExited reports whether
// the process has exited: gone, a zombie, or its PID reused.

// parseProcNetTCP returns the socket inodes of LISTEN entries in a
// /proc/net/tcp or /proc/net/tcp6 table that accept connections to endpoint.
func parseProcNetTCP(data []byte, endpoint netip.AddrPort) map[string]bool {
	inodes := map[string]bool{}
	want := endpoint.Addr().Unmap()
	for _, line := range bytes.Split(data, []byte("\n"))[1:] {
		fields := strings.Fields(string(line))
		if len(fields) < 10 || fields[3] != "0A" {
			continue
		}
		hostHex, portHex, ok := strings.Cut(fields[1], ":")
		if !ok {
			continue
		}
		port, err := strconv.ParseUint(portHex, 16, 16)
		if err != nil || uint16(port) != endpoint.Port() {
			continue
		}
		addr, ok := parseProcAddr(hostHex)
		if !ok {
			continue
		}
		addr = addr.Unmap()
		if addr == want || addr.IsUnspecified() && (addr.Is6() || addr.Is4() == want.Is4()) {
			inodes[fields[9]] = true
		}
	}
	return inodes
}

// parseProcAddr decodes the hex address of a /proc/net/tcp entry: the
// network-order bytes printed as native-endian 32-bit words.
func parseProcAddr(s string) (netip.Addr, bool) {
	raw, err := hex.DecodeString(s)
	if err != nil || (len(raw) != 4 && len(raw) != 16) {
		return netip.Addr{}, false
	}
	out := make([]byte, len(raw))
	for i := 0; i < len(raw); i += 4 {
		binary.NativeEndian.PutUint32(out[i:], binary.BigEndian.Uint32(raw[i:]))
	}
	addr, ok := netip.AddrFromSlice(out)
	return addr, ok
}

// parseProcStat returns the state and start time (clock ticks since boot)
// from a /proc/<pid>/stat line.
func parseProcStat(data []byte) (state byte, start uint64, err error) {
	i := bytes.LastIndexByte(data, ')')
	if i < 0 {
		return 0, 0, fmt.Errorf("malformed stat line")
	}
	fields := strings.Fields(string(data[i+1:]))
	// fields[0] is field 3 (state); field 22 (starttime) is fields[19].
	if len(fields) < 20 || len(fields[0]) != 1 {
		return 0, 0, fmt.Errorf("malformed stat line")
	}
	start, err = strconv.ParseUint(fields[19], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("malformed stat start time: %w", err)
	}
	return fields[0][0], start, nil
}
