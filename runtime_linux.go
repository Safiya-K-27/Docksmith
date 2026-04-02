//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

func runIsolated(rootfs, workdir string, envMap map[string]string, argv []string, shell bool) (int, error) {
	if len(argv) == 0 {
		return -1, fmt.Errorf("no command provided")
	}
	if os.Geteuid() != 0 {
		return -1, fmt.Errorf("linux isolation requires root privileges (run with sudo)")
	}
	rootAbs, err := filepath.Abs(rootfs)
	if err != nil {
		return -1, err
	}

	wd := toSlashClean(workdir)
	if wd == "" {
		wd = "/"
	}
	absWD := filepath.Join(rootAbs, filepath.FromSlash(strings.TrimPrefix(wd, "/")))
	if _, err := os.Stat(absWD); err != nil {
		return -1, fmt.Errorf("working directory %s does not exist in image root", wd)
	}

	script := ""
	if shell {
		script = fmt.Sprintf("cd %s && %s", shQuote(wd), strings.Join(argv, " "))
	} else {
		script = fmt.Sprintf("cd %s && exec %s", shQuote(wd), quoteArgs(argv))
	}
	cmd := exec.Command("/bin/sh", "-lc", script)

	env := buildIsolatedEnv(envMap)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = "/"
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Chroot: rootAbs,
		Cloneflags: syscall.CLONE_NEWNS | syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC | syscall.CLONE_NEWPID | syscall.CLONE_NEWNET,
		Unshareflags: syscall.CLONE_NEWNS,
		Pdeathsig: syscall.SIGKILL,
	}

	err = cmd.Run()
	if err == nil {
		return 0, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		if status, ok := ee.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus(), nil
		}
		return 1, nil
	}
	return -1, err
}

func quoteArgs(argv []string) string {
	out := make([]string, 0, len(argv))
	for _, a := range argv {
		out = append(out, shQuote(a))
	}
	return strings.Join(out, " ")
}

func shQuote(s string) string {
	return strconv.Quote(s)
}

func buildIsolatedEnv(in map[string]string) []string {
	merged := map[string]string{}
	for k, v := range in {
		merged[k] = v
	}
	if _, ok := merged["PATH"]; !ok {
		merged["PATH"] = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	if _, ok := merged["HOME"]; !ok {
		merged["HOME"] = "/root"
	}
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+merged[k])
	}
	return out
}
