package cmd

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/install"
	"github.com/Benehiko/jedi-outcast-coop/internal/paks"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
)

func newPk3Cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pk3",
		Short: "Build the override paks (co-op UI, co-op NPCs, widescreen menu)",
	}
	cmd.AddCommand(newPk3CoopUICmd(), newPk3CoopNPCsCmd(), newPk3WidescreenCmd())
	return cmd
}

func newPk3CoopUICmd() *cobra.Command {
	var out, repo string
	cmd := &cobra.Command{
		Use:   "coop-ui",
		Short: "Build the co-op UI overlay pak (zz-coop-ui.pk3)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := project.Root(repo)
			if err != nil {
				return err
			}
			src := filepath.Join(root, "assets", "coop-ui")
			if out == "" {
				out = filepath.Join(src, "zz-coop-ui.pk3")
			}
			names, err := paks.BuildCoopUI(src, out)
			if err != nil {
				return err
			}
			cmd.Printf("built %s\n", out)
			for _, n := range names {
				cmd.Printf("  %s\n", n)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&out, "out", "", "output pak (default: assets/coop-ui/zz-coop-ui.pk3)")
	cmd.Flags().StringVar(&repo, "repo", "", "repository root (default: autodetect from cwd)")
	return cmd
}

func newPk3CoopNPCsCmd() *cobra.Command {
	var outDir string
	cmd := &cobra.Command{
		Use:   "coop-npcs <path-to-GameData/base>",
		Short: "Build the co-op NPC compatibility pak from your retail assets0.pk3",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if outDir == "" {
				outDir = "."
			}
			res, err := paks.BuildCoopNPCs(args[0], outDir)
			if err != nil {
				return err
			}
			cmd.Printf("wrote %s (%d bytes, ~%d NPC definitions)\n", res.OutPath, res.Bytes, res.NumDefs)
			cmd.Println()
			cmd.Println("Install with:")
			cmd.Printf("  cp %q ~/.local/share/openjk/base/\n", res.OutPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&outDir, "out-dir", "", "output directory (default: current directory)")
	return cmd
}

func newPk3WidescreenCmd() *cobra.Command {
	var assets, out string
	cmd := &cobra.Command{
		Use:   "widescreen",
		Short: "Build the widescreen/QHD/ultrawide video-menu pak from your retail menus",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if assets == "" {
				assets = install.DefaultAssetsBase()
			}
			if out == "" {
				out = filepath.Join(assets, "zz-widescreen-menu.pk3")
			}
			res, err := paks.BuildWidescreen(assets, out)
			if err != nil {
				return err
			}
			cmd.Printf(">>> source menus: %s\n", res.SourcePak)
			for _, s := range res.Skipped {
				cmd.Printf(">>> skip %s\n", s)
			}
			for _, p := range res.Patched {
				cmd.Printf(">>> patched %s\n", p)
			}
			cmd.Printf("\n>>> built %s (%d menu file(s))\n", res.OutPath, len(res.Patched))
			cmd.Println("    SETUP > VIDEO > \"Video Mode\" now lists 720p through 5120x1440 (32:9).")
			cmd.Printf("    To remove: rm %q\n", res.OutPath)
			return nil
		},
	}
	cmd.Flags().StringVar(&assets, "assets", "", "directory containing your retail assets*.pk3 (default: platform base dir)")
	cmd.Flags().StringVar(&out, "out", "", "output pak (default: <assets>/zz-widescreen-menu.pk3)")
	return cmd
}
