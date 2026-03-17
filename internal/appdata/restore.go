package appdata

import (
	"fmt"
	"os"
	"runtime"

	"github.com/sourcegraph/conc/pool"
)

// RestoreResult holds the result of restoring a single app.
type RestoreResult struct {
	App     ResolvedApp
	Copied  int
	Skipped int
	Err     error
}

// restoreApp copies files from storage to home for a single app.
func restoreApp(app ResolvedApp, dryRun bool) RestoreResult {
	result := RestoreResult{App: app}

	for _, entry := range app.Entries {
		_, err := os.Stat(entry.StoragePath)
		if err != nil {
			if os.IsNotExist(err) {
				result.Skipped++
				continue
			}
			result.Err = fmt.Errorf("restore %s: stat %s: %w", app.ID, entry.StoragePath, err)
			return result
		}

		if dryRun {
			result.Copied++
			continue
		}

		if err := CopyPath(entry.StoragePath, entry.HomePath); err != nil {
			result.Err = fmt.Errorf("restore %s: %w", app.ID, err)
			return result
		}
		result.Copied++
	}

	return result
}

// RestoreAll restores all resolved apps concurrently.
func RestoreAll(apps []ResolvedApp, dryRun bool) []RestoreResult {
	p := pool.NewWithResults[RestoreResult]().WithMaxGoroutines(runtime.NumCPU())

	for _, app := range apps {
		p.Go(func() RestoreResult {
			return restoreApp(app, dryRun)
		})
	}

	return p.Wait()
}
