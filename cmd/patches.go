package cmd

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/Benehiko/jedi-outcast-coop/internal/patches"
	"github.com/Benehiko/jedi-outcast-coop/internal/project"
)

func newPatchesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "patches",
		Short: "Manage this repo's OpenJK patch set",
	}
	cmd.AddCommand(newPatchesApplyCmd())
	return cmd
}

func newPatchesApplyCmd() *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply the patch set to the pinned OpenJK submodule",
		Long: "Apply this repo's patches to the pinned OpenJK submodule, in order.\n\n" +
			"The patches are cumulative and overlap, so they must apply to a PRISTINE\n" +
			"submodule. If a patch fails, reset the submodule and re-run:\n" +
			"    git -C openjk checkout -- . && git -C openjk clean -fd",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := project.Root(repo)
			if err != nil {
				return err
			}
			a := &patches.Applier{
				Submodule:  filepath.Join(root, "openjk"),
				PatchesDir: filepath.Join(root, "patches"),
			}
			results, err := a.Apply(cmd.Context())
			for _, r := range results {
				if r.Status == patches.Skipped {
					cmd.Printf("skip    %s (already applied)\n", r.Name)
				} else {
					cmd.Printf("applied %s\n", r.Name)
				}
			}
			if err != nil {
				return err
			}
			if len(results) == 0 {
				cmd.Println("no patches to apply")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "repository root (default: autodetect from cwd)")
	return cmd
}
