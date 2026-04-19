package main

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type fileInfo struct {
	digest string
}

func tarSnapshot(storeRoot string, layers []LayerDescriptor) (map[string]fileInfo, error) {
	snap := map[string]fileInfo{}

	for _, layer := range layers {
		tarPath := digestFilePath(storeRoot, layer.Digest)
		f, err := os.Open(tarPath)
		if err != nil {
			return nil, fmt.Errorf("open layer %s: %w", layer.Digest, err)
		}

		tr := tar.NewReader(f)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				f.Close()
				return nil, fmt.Errorf("read layer %s: %w", layer.Digest, err)
			}

			// Normalize to /path/to/file
			name := strings.TrimSuffix(hdr.Name, "/")
			name = strings.TrimPrefix(name, "./")
			if !strings.HasPrefix(name, "/") {
				name = "/" + name
			}

			dir := path.Dir(name)
			base := path.Base(name)

			// Opaque whiteout: remove all existing entries in this directory
			if base == ".wh..wh..opq" {
				for p := range snap {
					if path.Dir(p) == dir {
						delete(snap, p)
					}
				}
				continue
			}

			// Whiteout: remove specific file
			if strings.HasPrefix(base, ".wh.") {
				actual := path.Join(dir, strings.TrimPrefix(base, ".wh."))
				delete(snap, actual)
				continue
			}

			switch hdr.Typeflag {
			case tar.TypeReg, tar.TypeRegA:
				h := sha256.New()
				if _, err := io.Copy(h, tr); err != nil {
					f.Close()
					return nil, fmt.Errorf("hash %s in layer %s: %w", hdr.Name, layer.Digest, err)
				}
				snap[name] = fileInfo{digest: hex.EncodeToString(h.Sum(nil))}
			case tar.TypeSymlink:
				snap[name] = fileInfo{digest: "symlink:" + hdr.Linkname}
			}
		}
		f.Close()
	}

	return snap, nil
}

func cmdDiff(storeRoot string, ref1, ref2 ImageRef) error {
	m1, err := loadManifest(storeRoot, ref1)
	if err != nil {
		return err
	}
	m2, err := loadManifest(storeRoot, ref2)
	if err != nil {
		return err
	}

	snap1, err := tarSnapshot(storeRoot, m1.Layers)
	if err != nil {
		return fmt.Errorf("snapshot %s:%s: %w", ref1.Name, ref1.Tag, err)
	}
	snap2, err := tarSnapshot(storeRoot, m2.Layers)
	if err != nil {
		return fmt.Errorf("snapshot %s:%s: %w", ref2.Name, ref2.Tag, err)
	}

	type diffEntry struct {
		filePath string
		status   string
	}
	var results []diffEntry

	for p, fi2 := range snap2 {
		if fi1, ok := snap1[p]; !ok {
			results = append(results, diffEntry{p, "added"})
		} else if fi1.digest != fi2.digest {
			results = append(results, diffEntry{p, "modified"})
		}
	}
	for p := range snap1 {
		if _, ok := snap2[p]; !ok {
			results = append(results, diffEntry{p, "removed"})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].filePath < results[j].filePath
	})

	prefix := map[string]string{"added": "+", "modified": "~", "removed": "-"}
	for _, r := range results {
		fmt.Printf("%s %s (%s)\n", prefix[r.status], r.filePath, r.status)
	}
	return nil
}

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
