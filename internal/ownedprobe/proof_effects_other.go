//go:build !linux

package ownedprobe

// No fallback native effects on unsupported operating systems.
func realProofOps() nativeProofOps { return nativeProofOps{} }
