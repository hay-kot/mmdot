package appdata

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const snapshotDir = ".snapshots"

// SnapshotHomeFiles creates a zip archive of the home-side files that are about
// to be overwritten by a restore. Only files that currently exist are included.
// The snapshot is stored in storageDir/.snapshots/.
func SnapshotHomeFiles(storageDir string, apps []ResolvedApp) (string, error) {
	snapDir := filepath.Join(storageDir, snapshotDir)
	if err := os.MkdirAll(snapDir, 0755); err != nil {
		return "", fmt.Errorf("create snapshot dir: %w", err)
	}

	name := time.Now().Format("2006-01-02T15-04-05") + ".zip"
	snapPath := filepath.Join(snapDir, name)

	f, err := os.Create(snapPath)
	if err != nil {
		return "", fmt.Errorf("create snapshot file: %w", err)
	}

	w := zip.NewWriter(f)

	var walkErr error
	for _, app := range apps {
		for _, entry := range app.Entries {
			info, err := os.Lstat(entry.HomePath)
			if err != nil {
				continue // file doesn't exist, nothing to snapshot
			}
			if !info.Mode().IsRegular() {
				continue // skip symlinks, dirs, etc.
			}

			arcPath := app.ID + "/" + filepath.Base(entry.HomePath)
			if err := addFileToZip(w, entry.HomePath, arcPath); err != nil {
				walkErr = fmt.Errorf("snapshot %s: %w", entry.HomePath, err)
				break
			}
		}
		if walkErr != nil {
			break
		}
	}

	// Close writer then file explicitly to catch flush errors
	if closeErr := w.Close(); closeErr != nil && walkErr == nil {
		walkErr = fmt.Errorf("finalize snapshot: %w", closeErr)
	}
	if closeErr := f.Close(); closeErr != nil && walkErr == nil {
		walkErr = fmt.Errorf("close snapshot file: %w", closeErr)
	}

	if walkErr != nil {
		_ = os.Remove(snapPath)
		return "", walkErr
	}

	return snapPath, nil
}

func addFileToZip(w *zip.Writer, srcPath, arcPath string) (rerr error) {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := src.Close(); cerr != nil && rerr == nil {
			rerr = cerr
		}
	}()

	fw, err := w.Create(arcPath)
	if err != nil {
		return err
	}

	_, err = io.Copy(fw, src)
	return err
}

// PruneSnapshots keeps the most recent `keep` snapshots and removes older ones.
func PruneSnapshots(storageDir string, keep int) error {
	if keep <= 0 {
		return nil
	}

	snapDir := filepath.Join(storageDir, snapshotDir)
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read snapshot dir: %w", err)
	}

	// Filter to .zip files only
	var zips []os.DirEntry
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".zip") {
			zips = append(zips, e)
		}
	}

	if len(zips) <= keep {
		return nil
	}

	// Sort by name (timestamp format ensures chronological order)
	sort.Slice(zips, func(i, j int) bool {
		return zips[i].Name() < zips[j].Name()
	})

	// Remove oldest
	for _, e := range zips[:len(zips)-keep] {
		path := filepath.Join(snapDir, e.Name())
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("remove snapshot %s: %w", e.Name(), err)
		}
	}

	return nil
}
