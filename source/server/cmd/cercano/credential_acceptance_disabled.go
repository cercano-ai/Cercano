//go:build !realcloudacceptance || !darwin || !cgo

package main

// Reserved command fails closed without starting the app on other builds.
func queryAcceptanceCredential() int { return -4 }
