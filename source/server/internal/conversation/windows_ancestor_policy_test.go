package conversation

import "testing"

func TestWindowsAncestorOwnerPolicyPortable(t *testing.T) {
	const user = "S-1-5-21-1-2-3-1005"
	for _, tc := range []struct {
		owner string
		ok    bool
	}{
		{user, true}, {"S-1-5-18", true}, {"S-1-5-32-544", true},
		{"S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464", true},
		{"S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478465", false},
		{"S-1-5-80-1", false}, {"S-1-5-21-1-2-3-1006", false},
	} {
		t.Run(tc.owner, func(t *testing.T) {
			if got := windowsAncestorOwnerTrusted(tc.owner, user); got != tc.ok {
				t.Fatalf("trusted=%v, want %v", got, tc.ok)
			}
		})
	}
}

func TestWindowsAncestorRightsPolicyPortable(t *testing.T) {
	for _, tc := range []struct {
		name string
		mask uint32
		ok   bool
	}{
		{"read-traverse", 0x1200a9, true},
		{"create-file-sibling", 0x2, true},
		{"create-directory-sibling", 0x4, true},
		{"read-and-create-siblings", 0x1200af, true},
		{"delete-child", 0x40, false}, {"delete-self", 0x10000, false},
		{"write-dacl", 0x40000, false}, {"write-owner", 0x80000, false},
		{"create-and-delete-child", 0x46, false},
		{"modify-and-delete-child", 0x1301ff, false},
		{"generic-write", 0x40000000, false}, {"generic-all", 0x10000000, false},
		{"write-attributes", 0x100, false}, {"unknown", 0x800000, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.mask
			if got := windowsAncestorUnrelatedRightsSafe(tc.mask); got != tc.ok {
				t.Fatalf("safe=%v, want %v (mask %#x)", got, tc.ok, tc.mask)
			}
			if tc.mask != before {
				t.Fatal("mutated mask")
			}
		})
	}
}
