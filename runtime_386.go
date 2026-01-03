//go:build 386

package main

import "runtime"

func init() {
	// iSH app (iOS) emulates x86 in usermode and performs poorly with
	// multiple goroutines. Force single-threaded execution for 386 builds.
	runtime.GOMAXPROCS(1)
}
