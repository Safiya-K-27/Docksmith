//go:build !linux

package main

import "fmt"

func runIsolated(rootfs, workdir string, envMap map[string]string, argv []string, shell bool) (int, error) {
	return -1, fmt.Errorf("docksmith runtime requires Linux")
}
