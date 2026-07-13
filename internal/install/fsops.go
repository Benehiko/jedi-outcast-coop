package install

import "os"

// replaceSymlink creates (or atomically refreshes) a symlink at linkPath
// pointing to target, like `ln -sfn`. An existing link/file at linkPath is
// removed first so re-runs are idempotent.
//
// On Windows this requires either Developer Mode or elevation; the caller
// surfaces the error to the user if the OS refuses.
func replaceSymlink(target, linkPath string) error {
	if _, err := os.Lstat(linkPath); err == nil {
		if err := os.Remove(linkPath); err != nil {
			return err
		}
	}
	return os.Symlink(target, linkPath)
}
