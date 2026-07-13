//go:build !windows

package launch

import (
	"os"
	"path/filepath"
	"syscall"

	"github.com/Benehiko/jedi-outcast-coop/internal/install"
)

// baseName is the engine executable's basename, used for argv[0].
func baseName(bin string) string { return filepath.Base(bin) }

// Run resolves and execs the engine, replacing the current process so the game
// runs directly under the caller's shell (it keeps running after jk2coop would
// have exited). It only returns on error — on success control never comes back.
func Run(p install.Platform, opts *Options) error {
	bin, args, err := Resolve(p, opts)
	if err != nil {
		return err
	}
	// argv[0] is the program name by convention; the rest are the engine args.
	argv := append([]string{baseName(bin)}, args...)
	return syscall.Exec(bin, argv, os.Environ())
}
