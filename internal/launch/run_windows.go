//go:build windows

package launch

import (
	"os"
	"os/exec"

	"github.com/Benehiko/jedi-outcast-coop/internal/install"
)

// Run resolves and starts the engine, then waits for it to exit. Windows has no
// exec(2) to replace the process in place, so the engine runs as a child with
// the console's stdio wired through.
func Run(p install.Platform, opts *Options) error {
	bin, args, err := Resolve(p, opts)
	if err != nil {
		return err
	}
	c := exec.Command(bin, args...)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
