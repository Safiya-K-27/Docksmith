package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type buildState struct {
	baseManifest ImageManifest
	layers       []LayerDescriptor
	config       ImageConfig
	workdirDirty bool
}

func buildImage(storeRoot string, opts BuildOptions) error {
	instructions, err := parseDocksmithfile(opts.Context)
	if err != nil {
		return err
	}
	idx, err := loadCacheIndex(storeRoot)
	if err != nil {
		return err
	}

	state := buildState{config: ImageConfig{Env: map[string]string{}, WorkingDir: "/"}}
	cacheCascadeMiss := false
	prevProducingDigest := ""
	stepNum := 0

	for _, ins := range instructions {
		stepNum++
		start := time.Now()
		fmt.Printf("STEP %d: %s\n", stepNum, ins.Raw)

		switch ins.Op {
		case "FROM":
			ref, err := parseImageRef(ins.Args)
			if err != nil {
				return fmt.Errorf("Docksmithfile:%d: %w", ins.Line, err)
			}
			base, err := loadManifest(storeRoot, ref)
			if err != nil {
				return fmt.Errorf("Docksmithfile:%d: %w", ins.Line, err)
			}
			state.baseManifest = base
			state.layers = append([]LayerDescriptor{}, base.Layers...)
			state.config = ImageConfig{Env: map[string]string{}, Cmd: nil, WorkingDir: "/"}
			for k, v := range base.Config.Env {
				state.config.Env[k] = v
			}
			if len(base.Config.Cmd) > 0 {
				state.config.Cmd = append([]string{}, base.Config.Cmd...)
			}
			if base.Config.WorkingDir != "" {
				state.config.WorkingDir = base.Config.WorkingDir
			}
			prevProducingDigest = ""
			continue

		case "WORKDIR":
			state.config.WorkingDir = toSlashClean(ins.Args)
			state.workdirDirty = true
			continue

		case "ENV":
			kv := strings.SplitN(ins.Args, "=", 2)
			if len(kv) != 2 || kv[0] == "" {
				return fmt.Errorf("Docksmithfile:%d: ENV must be key=value", ins.Line)
			}
			state.config.Env[kv[0]] = kv[1]
			continue

		case "CMD":
			var cmd []string
			if err := json.Unmarshal([]byte(ins.Args), &cmd); err != nil {
				return fmt.Errorf("Docksmithfile:%d: CMD must be JSON array", ins.Line)
			}
			state.config.Cmd = cmd
			continue
		}

		cacheKey, err := computeCacheKey(opts.Context, state, ins, prevProducingDigest)
		if err != nil {
			return fmt.Errorf("Docksmithfile:%d: %w", ins.Line, err)
		}

		layerDigestFromCache := ""
		cacheHit := false
		if !opts.NoCache && !cacheCascadeMiss {
			if d, ok := idx.Entries[cacheKey]; ok {
				if _, err := os.Stat(digestFilePath(storeRoot, d)); err == nil {
					cacheHit = true
					layerDigestFromCache = d
				}
			}
		}

		if cacheHit {
			fmt.Printf("  [CACHE HIT] (%s)\n", time.Since(start).Round(time.Millisecond))
			fi, err := os.Stat(digestFilePath(storeRoot, layerDigestFromCache))
			if err != nil {
				return err
			}
			state.layers = append(state.layers, LayerDescriptor{Digest: layerDigestFromCache, Size: fi.Size(), CreatedBy: ins.Raw})
			prevProducingDigest = layerDigestFromCache
			continue
		}

		cacheCascadeMiss = true
		fmt.Printf("  [CACHE MISS]\n")
		layer, err := executeProducingStep(storeRoot, opts.Context, state, ins)
		if err != nil {
			return fmt.Errorf("Docksmithfile:%d: %w", ins.Line, err)
		}
		layer.CreatedBy = ins.Raw
		state.layers = append(state.layers, layer)
		prevProducingDigest = layer.Digest
		state.workdirDirty = false
		if !opts.NoCache {
			idx.Entries[cacheKey] = layer.Digest
		}
		fmt.Printf("  finished in %s\n", time.Since(start).Round(time.Millisecond))
	}

	if state.baseManifest.Name == "" {
		return fmt.Errorf("build has no FROM base image")
	}

	created := ""
	if existing, err := loadManifest(storeRoot, opts.TagRef); err == nil {
		created = existing.Created
	}
	if created == "" {
		created = currentTimestamp()
	}

	manifest := ImageManifest{
		Name:    opts.TagRef.Name,
		Tag:     opts.TagRef.Tag,
		Created: created,
		Config: ImageConfig{
			Env:        map[string]string{},
			Cmd:        append([]string{}, state.config.Cmd...),
			WorkingDir: state.config.WorkingDir,
		},
		Layers: state.layers,
	}
	for k, v := range state.config.Env {
		manifest.Config.Env[k] = v
	}

	if err := saveManifest(storeRoot, manifest); err != nil {
		return err
	}
	if !opts.NoCache {
		if err := saveCacheIndex(storeRoot, idx); err != nil {
			return err
		}
	}
	fmt.Printf("Built image %s:%s\n", opts.TagRef.Name, opts.TagRef.Tag)
	return nil
}

func computeCacheKey(context string, state buildState, ins Instruction, prevProducingDigest string) (string, error) {
	base := prevProducingDigest
	if base == "" {
		base = state.baseManifest.Digest
	}
	if base == "" {
		return "", fmt.Errorf("missing base digest")
	}
	parts := []string{
		"base=" + base,
		"instruction=" + ins.Raw,
		"workdir=" + state.config.WorkingDir,
	}
	parts = append(parts, "env="+strings.Join(sortedEnv(state.config.Env), "\n"))

	if ins.Op == "COPY" {
		copyParts, err := copyArgs(ins.Args)
		if err != nil {
			return "", err
		}
		srcs, err := expandCopySources(context, copyParts[0])
		if err != nil {
			return "", err
		}
		hashes := make([]string, 0, len(srcs))
		for _, s := range srcs {
			fi, err := os.Lstat(s)
			if err != nil {
				return "", err
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("COPY does not allow symlink source %q", s)
			}
			if fi.IsDir() {
				err := filepath.Walk(s, func(p string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if info.IsDir() {
						return nil
					}
					if info.Mode()&os.ModeSymlink != 0 {
						return fmt.Errorf("COPY does not allow symlink source %q", p)
					}
					rel, err := filepath.Rel(context, p)
					if err != nil {
						return err
					}
					rel = filepath.ToSlash(rel)
					h, err := sha256File(p)
					if err != nil {
						return err
					}
					hashes = append(hashes, rel+":"+h)
					return nil
				})
				if err != nil {
					return "", err
				}
				continue
			}
			rel, err := filepath.Rel(context, s)
			if err != nil {
				return "", err
			}
			rel = filepath.ToSlash(rel)
			h, err := sha256File(s)
			if err != nil {
				return "", err
			}
			hashes = append(hashes, rel+":"+h)
		}
		sort.Strings(hashes)
		parts = append(parts, "copyhashes="+strings.Join(hashes, "|"))
	}

	return sha256Bytes([]byte(strings.Join(parts, "\n"))), nil
}

func copyArgs(args string) ([2]string, error) {
	fields := strings.Fields(args)
	if len(fields) != 2 {
		return [2]string{}, fmt.Errorf("COPY requires src and dest")
	}
	return [2]string{fields[0], fields[1]}, nil
}

func executeProducingStep(storeRoot, context string, state buildState, ins Instruction) (LayerDescriptor, error) {
	rootfs, err := os.MkdirTemp("", "docksmith-build-rootfs-")
	if err != nil {
		return LayerDescriptor{}, err
	}
	defer os.RemoveAll(rootfs)

	layerDigests := make([]string, 0, len(state.layers))
	for _, l := range state.layers {
		layerDigests = append(layerDigests, l.Digest)
	}
	if err := extractLayers(rootfs, layerDigests, storeRoot); err != nil {
		return LayerDescriptor{}, err
	}

	if state.workdirDirty {
		wd := filepath.Join(rootfs, filepath.FromSlash(strings.TrimPrefix(state.config.WorkingDir, "/")))
		if err := os.MkdirAll(wd, 0o755); err != nil {
			return LayerDescriptor{}, err
		}
	}

	before, err := snapshotTree(rootfs)
	if err != nil {
		return LayerDescriptor{}, err
	}

	switch ins.Op {
	case "COPY":
		args, err := copyArgs(ins.Args)
		if err != nil {
			return LayerDescriptor{}, err
		}
		if err := applyCopy(context, rootfs, args[0], args[1]); err != nil {
			return LayerDescriptor{}, err
		}
	case "RUN":
		exitCode, err := runIsolated(rootfs, state.config.WorkingDir, state.config.Env, []string{ins.Args}, true)
		if err != nil {
			return LayerDescriptor{}, err
		}
		if exitCode != 0 {
			return LayerDescriptor{}, fmt.Errorf("RUN command exited with code %d", exitCode)
		}
	default:
		return LayerDescriptor{}, fmt.Errorf("unsupported producing op %s", ins.Op)
	}

	after, err := snapshotTree(rootfs)
	if err != nil {
		return LayerDescriptor{}, err
	}
	changed := changedPaths(before, after)
	tarBytes, err := buildDeltaTar(rootfs, changed)
	if err != nil {
		return LayerDescriptor{}, err
	}
	layer, err := storeLayer(storeRoot, tarBytes)
	if err != nil {
		return LayerDescriptor{}, err
	}
	return layer, nil
}

func applyCopy(context, rootfs, srcPattern, dest string) error {
	sources, err := expandCopySources(context, srcPattern)
	if err != nil {
		return err
	}
	dest = toSlashClean(dest)
	destInRoot := filepath.Join(rootfs, filepath.FromSlash(strings.TrimPrefix(dest, "/")))

	if len(sources) > 1 {
		if err := os.MkdirAll(destInRoot, 0o755); err != nil {
			return err
		}
	}

	for _, src := range sources {
		info, err := os.Lstat(src)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("COPY does not allow symlink source %q", src)
		}
		baseName := filepath.Base(src)
		target := destInRoot
		if info.IsDir() || len(sources) > 1 || strings.HasSuffix(dest, "/") {
			target = filepath.Join(destInRoot, baseName)
		}
		if info.IsDir() {
			if err := copyDir(src, target); err != nil {
				return err
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := copyFile(src, target, info.Mode().Perm()); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = out.ReadFrom(in)
	return err
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("COPY does not allow symlink source %q", path)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}
