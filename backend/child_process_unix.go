//go:build !windows

package main

import "os/exec"

func configureChildProcess(cmd *exec.Cmd) {}
