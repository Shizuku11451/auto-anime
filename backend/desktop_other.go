//go:build !windows

package main

func desktopRequested() bool { return false }
func runDesktop()            {}

func acquireApplicationLock(string) (func(), error) { return func() {}, nil }
