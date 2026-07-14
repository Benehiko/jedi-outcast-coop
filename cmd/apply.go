package cmd

import (
	"context"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/config"
	"github.com/Benehiko/jedi-outcast-coop/internal/gfxprobe"
	"github.com/Benehiko/jedi-outcast-coop/internal/install"
)

// refreshAutoexec rewrites the installed base/autoexec_sp.cfg from the config so
// a settings change takes effect on the next launch without a full reinstall.
// It is best-effort: if nothing is installed yet (no base dir), it quietly does
// nothing — the next `jk2coop install` will write it.
//
// Before writing it clamps the config's MSAA to a level this GPU/driver can
// actually realise (some Mesa/Wayland stacks fail eglChooseConfig at high sample
// counts, which crashes the renderer at launch with a misleading "no display
// modes could be found"). This is the guard on the launch path too: every
// launch verb refreshes autoexec through here first, so a stale unsupported MSAA
// is stepped down before the engine ever sees it. A change is persisted back to
// config.toml so the fix is sticky and the next run needs no re-probe.
func refreshAutoexec(cmd *cobra.Command, cfg config.Config) {
	buildDir := install.EnvOr("JK2_BUILD", "")
	p := install.DetectPlatform(buildDir)
	baseDir := p.BaseDir()
	if _, err := os.Stat(baseDir); err != nil {
		return // not installed yet
	}

	if usable, changed := gfxprobe.ClampMSAA(
		cmd.Context(), p, buildDir, cfg.Graphics.MSAA,
	); changed {
		cmd.Printf("note: %s MSAA is unsupported on this GPU/driver; using %s instead.\n",
			msaaLabel(cfg.Graphics.MSAA), msaaLabel(usable))
		cfg.Graphics.MSAA = usable
		if path, err := cfg.Save(); err != nil {
			cmd.Printf("note: could not persist the MSAA change (%v)\n", err)
		} else {
			cmd.Printf("updated %s\n", path)
		}
	}

	cfgPath := filepath.Join(baseDir, "autoexec_sp.cfg")
	if err := os.WriteFile(cfgPath, cfg.AutoexecBytes(), 0o644); err != nil {
		cmd.Printf("note: could not refresh autoexec (%v); run `jk2coop install`\n", err)
		return
	}
	cmd.Println("refreshed autoexec_sp.cfg; takes effect on next launch.")
}

// promptYN asks a y/N question on the terminal, defaulting to no off a TTY.
func promptYN(cmd *cobra.Command, question string) bool {
	ask := stdinPrompt(cmd)
	if ask == nil {
		return false
	}
	yes, err := ask(question)
	return err == nil && yes
}

// offerRebuild asks whether to rebuild the engine + reinstall now (used by the
// graphics menu when a patch-backed feature changed). Returns nil if the user
// declines. On yes it runs a full install, which rebuilds the engine to match
// the config and re-stages everything.
func offerRebuild(cmd *cobra.Command, root, buildDir string, cfg config.Config) error {
	if !promptYN(cmd, "Rebuild the engine and reinstall now? (slow)") {
		cmd.Println("skipped; run `jk2coop install` when ready to apply the graphics change.")
		return nil
	}
	if buildDir == "" {
		buildDir = install.EnvOr("JK2_BUILD", filepath.Join(root, "openjk", "build"))
	}
	p := install.DetectPlatform(buildDir)
	opts := &install.Options{
		RepoRoot:  root,
		BuildDir:  buildDir,
		Config:    &cfg,
		AssumeYes: true,
		Out:       cmd.OutOrStdout(),
		Prompt:    stdinPrompt(cmd),
	}
	return install.Install(context.Background(), p, opts)
}
