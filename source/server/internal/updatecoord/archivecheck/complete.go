package archivecheck

// CompleteReleaseLayout is the explicit three-binary contract for the native
// temporary-install caller. Generic Layout callers retain their own allowlists;
// adding the updater here does not silently admit it to older package formats.
func CompleteReleaseLayout() Layout {
	return Layout{Required: []string{"bin/cercano", "bin/cercano-cli", "bin/cercano-updater"}}
}
