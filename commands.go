package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cmdImages(storeRoot string) error {
	imgs, err := listManifests(storeRoot)
	if err != nil {
		return err
	}
	fmt.Printf("%-24s %-12s %-14s %-24s\n", "NAME", "TAG", "ID", "CREATED")
	for _, m := range imgs {
		id := strings.TrimPrefix(m.Digest, "sha256:")
		if len(id) > 12 {
			id = id[:12]
		}
		fmt.Printf("%-24s %-12s %-14s %-24s\n", m.Name, m.Tag, id, m.Created)
	}
	return nil
}

func cmdRmi(storeRoot string, ref ImageRef) error {
	m, err := loadManifest(storeRoot, ref)
	if err != nil {
		return err
	}
	if err := os.Remove(imageManifestFile(storeRoot, ref)); err != nil {
		return err
	}
	for _, l := range m.Layers {
		_ = os.Remove(digestFilePath(storeRoot, l.Digest))
	}
	fmt.Printf("Removed image %s:%s\n", ref.Name, ref.Tag)
	return nil
}

func cmdRun(storeRoot string, opts RuntimeOptions) error {
	m, err := loadManifest(storeRoot, opts.ImageRef)
	if err != nil {
		return err
	}
	argv := opts.OverrideCmd
	if len(argv) == 0 {
		argv = m.Config.Cmd
	}
	if len(argv) == 0 {
		return fmt.Errorf("no command provided and image has no CMD")
	}

	rootfs, err := os.MkdirTemp("", "docksmith-run-rootfs-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(rootfs)

	var digests []string
	for _, l := range m.Layers {
		digests = append(digests, l.Digest)
	}
	if err := extractLayers(rootfs, digests, storeRoot); err != nil {
		return err
	}

	env := map[string]string{}
	for k, v := range m.Config.Env {
		env[k] = v
	}
	for k, v := range opts.OverrideEnv {
		env[k] = v
	}
	wd := m.Config.WorkingDir
	if wd == "" {
		wd = "/"
	}
	exitCode, err := runIsolated(rootfs, wd, env, argv, false)
	if err != nil {
		return err
	}
	fmt.Printf("Container exited with code %d\n", exitCode)
	if exitCode != 0 {
		return fmt.Errorf("container failed")
	}
	return nil
}

func cmdImportRootfs(storeRoot string, ref ImageRef, rootfsDir string) error {
	rootfsDirAbs, err := filepath.Abs(rootfsDir)
	if err != nil {
		return err
	}
	if st, err := os.Stat(rootfsDirAbs); err != nil || !st.IsDir() {
		return fmt.Errorf("rootfs directory does not exist: %s", rootfsDir)
	}

	snap, err := snapshotTree(rootfsDirAbs)
	if err != nil {
		return err
	}
	var all []string
	for p := range snap {
		all = append(all, p)
	}
	sortStrings(all)
	tarBytes, err := buildDeltaTar(rootfsDirAbs, all)
	if err != nil {
		return err
	}
	layer, err := storeLayer(storeRoot, tarBytes)
	if err != nil {
		return err
	}
	layer.CreatedBy = "import-rootfs"

	created := currentTimestamp()
	if existing, err := loadManifest(storeRoot, ref); err == nil {
		created = existing.Created
	}
	m := ImageManifest{
		Name:    ref.Name,
		Tag:     ref.Tag,
		Created: created,
		Config: ImageConfig{
			Env:        map[string]string{},
			Cmd:        nil,
			WorkingDir: "/",
		},
		Layers: []LayerDescriptor{layer},
	}
	if err := saveManifest(storeRoot, m); err != nil {
		return err
	}
	fmt.Printf("Imported base image %s:%s\n", ref.Name, ref.Tag)
	return nil
}

func sortStrings(v []string) {
	for i := 0; i < len(v); i++ {
		for j := i + 1; j < len(v); j++ {
			if v[j] < v[i] {
				v[i], v[j] = v[j], v[i]
			}
		}
	}
}
