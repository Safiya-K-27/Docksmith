package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var validOps = map[string]bool{
	"FROM":    true,
	"COPY":    true,
	"RUN":     true,
	"WORKDIR": true,
	"ENV":     true,
	"CMD":     true,
}

func parseDocksmithfile(contextDir string) ([]Instruction, error) {
	path := filepath.Join(contextDir, "Docksmithfile")
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open Docksmithfile: %w", err)
	}
	defer f.Close()

	var out []Instruction
	s := bufio.NewScanner(f)
	lineNo := 0
	for s.Scan() {
		lineNo++
		raw := strings.TrimSpace(s.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		parts := strings.SplitN(raw, " ", 2)
		op := strings.ToUpper(strings.TrimSpace(parts[0]))
		args := ""
		if len(parts) > 1 {
			args = strings.TrimSpace(parts[1])
		}
		if !validOps[op] {
			return nil, fmt.Errorf("Docksmithfile:%d: unknown instruction %q", lineNo, op)
		}
		if args == "" {
			return nil, fmt.Errorf("Docksmithfile:%d: missing arguments for %s", lineNo, op)
		}
		if op == "CMD" {
			var arr []string
			if err := json.Unmarshal([]byte(args), &arr); err != nil {
				return nil, fmt.Errorf("Docksmithfile:%d: CMD must be JSON string array", lineNo)
			}
		}
		if op == "ENV" {
			if !strings.Contains(args, "=") {
				return nil, fmt.Errorf("Docksmithfile:%d: ENV must be key=value", lineNo)
			}
		}
		if op == "COPY" {
			p := strings.Fields(args)
			if len(p) != 2 {
				return nil, fmt.Errorf("Docksmithfile:%d: COPY requires src and dest", lineNo)
			}
		}
		out = append(out, Instruction{Op: op, Args: args, Raw: raw, Line: lineNo})
	}
	if err := s.Err(); err != nil {
		return nil, fmt.Errorf("failed reading Docksmithfile: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("Docksmithfile has no instructions")
	}
	if out[0].Op != "FROM" {
		return nil, fmt.Errorf("Docksmithfile:%d: first instruction must be FROM", out[0].Line)
	}
	return out, nil
}
