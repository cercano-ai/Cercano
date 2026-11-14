package conversation

// These predicates are platform-independent so the Windows policy can also be
// regression-tested on build hosts. The native descriptor validator calls them.
// Exact NT SERVICE\TrustedInstaller identity, not the service-SID namespace.
// This is only an ancestor-owner exception, never a protected-object trustee.
const windowsTrustedInstallerSID = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"

func windowsAncestorOwnerTrusted(owner, current string) bool {
	return owner == current || owner == "S-1-5-18" || owner == "S-1-5-32-544" || owner == windowsTrustedInstallerSID
}

func windowsAncestorUnrelatedRightsSafe(mask uint32) bool {
	const readTraverse = uint32(0x80000000 | 0x20000000 | 0x00100000 | 0x00020000 | 0x1 | 0x8 | 0x20 | 0x80)
	// FILE_ADD_FILE / FILE_ADD_SUBDIRECTORY create unrelated siblings; they
	// cannot replace an existing protected name without deletion rights. Every
	// child is independently validated before use. Keep DELETE, DELETE_CHILD,
	// WRITE_DAC/OWNER, generic write/all and unknown rights fail-closed.
	const createSibling = uint32(0x2 | 0x4)
	return mask & ^(readTraverse|createSibling) == 0
}
