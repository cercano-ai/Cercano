package main

// Windows has no Unix socket path budget; honor the OS temp parent.
func developmentProductionTempParent() string { return "" }
