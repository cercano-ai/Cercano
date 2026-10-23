package brewrestart

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"testing"
)

// procAddr4 formats an IPv4 address the way /proc/net/tcp prints it: the
// network-order bytes read as one native-endian word.
func procAddr4(a netip.Addr) string {
	b := a.As4()
	return fmt.Sprintf("%08X", binary.NativeEndian.Uint32(b[:]))
}

func procAddr6(a netip.Addr) string {
	b := a.As16()
	s := ""
	for i := 0; i < 16; i += 4 {
		s += fmt.Sprintf("%08X", binary.NativeEndian.Uint32(b[i:i+4]))
	}
	return s
}

func TestParseProcNetTCPFindsListeners(t *testing.T) {
	loop := netip.MustParseAddr("127.0.0.1")
	any4 := netip.MustParseAddr("0.0.0.0")
	other := netip.MustParseAddr("10.0.0.5")
	tcp := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		fmt.Sprintf("   0: %s:C352 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 111 1 0 100 0 0 10 0\n", procAddr4(loop)) +
		fmt.Sprintf("   1: %s:C352 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 222 1 0 100 0 0 10 0\n", procAddr4(any4)) +
		fmt.Sprintf("   2: %s:C352 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 333 1 0 100 0 0 10 0\n", procAddr4(other)) +
		fmt.Sprintf("   3: %s:C352 %s:D000 01 00000000:00000000 00:00000000 00000000  1000        0 444 1 0 100 0 0 10 0\n", procAddr4(loop), procAddr4(loop)) +
		fmt.Sprintf("   4: %s:C353 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 555 1 0 100 0 0 10 0\n", procAddr4(loop))
	got := parseProcNetTCP([]byte(tcp), netip.AddrPortFrom(loop, 0xC352))
	if !got["111"] || !got["222"] || got["333"] || got["444"] || got["555"] || len(got) != 2 {
		t.Fatalf("inodes = %v, want 111 and 222", got)
	}

	tcp6 := "  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		fmt.Sprintf("   0: %s:C352 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 666 1 0 100 0 0 10 0\n", procAddr6(netip.IPv6Unspecified())) +
		fmt.Sprintf("   1: %s:C352 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 777 1 0 100 0 0 10 0\n", procAddr6(netip.MustParseAddr("::ffff:127.0.0.1"))) +
		fmt.Sprintf("   2: %s:C352 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 888 1 0 100 0 0 10 0\n", procAddr6(netip.MustParseAddr("fe80::1")))
	got = parseProcNetTCP([]byte(tcp6), netip.AddrPortFrom(loop, 0xC352))
	if !got["666"] || !got["777"] || got["888"] || len(got) != 2 {
		t.Fatalf("tcp6 inodes = %v, want 666 and 777", got)
	}
}

func TestParseProcStat(t *testing.T) {
	// comm may contain spaces and parentheses.
	stat := "4242 (cer (cano) x) S 1 4242 4242 0 -1 4194560 100 0 0 0 1 2 0 0 20 0 9 0 987654 1000 10 18446744073709551615"
	state, start, err := parseProcStat([]byte(stat))
	if err != nil || state != 'S' || start != 987654 {
		t.Fatalf("parseProcStat = %q, %d, %v", state, start, err)
	}
	if _, _, err := parseProcStat([]byte("4242 (x) Z 1")); err == nil {
		t.Fatal("short stat line accepted")
	}
}
