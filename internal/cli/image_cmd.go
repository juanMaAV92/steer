package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/juanMaAV92/steer/internal/render"
	"github.com/spf13/cobra"
)

// NewImageCmd agrupa los subcomandos de la capacidad de imágenes (registry).
func NewImageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "image",
		Aliases: []string{"img"},
		Short:   "Browse container images in the context registry",
	}
	cmd.AddCommand(newImageLsCmd(), newImageTagsCmd())
	return cmd
}

func newImageLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List repositories with their latest tag",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := FromContext(cmd.Context())
			reg, err := app.Registry(cmd.Context())
			if err != nil {
				return err
			}
			repos, err := reg.ListRepositories(cmd.Context())
			if err != nil {
				return err
			}
			now := time.Now()
			rows := make([][]string, 0, len(repos))
			for _, r := range repos {
				latest, pushed := "—", "—"
				if tags, err := reg.ListTags(cmd.Context(), r.Name); err == nil && len(tags) > 0 {
					latest = tags[0].Tag
					pushed = render.Age(tags[0].PushedAt, now)
				}
				short := strings.TrimPrefix(r.Name, app.Ctx.RepoPrefix())
				rows = append(rows, []string{short, latest, pushed})
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), render.Table([]string{"REPO", "LATEST TAG", "PUSHED"}, rows))
			return nil
		},
	}
}

func newImageTagsCmd() *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:   "tags",
		Short: "List deployable image tags of a repository",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repo == "" {
				return fmt.Errorf("--repo is required (short name, e.g. -r api)")
			}
			app := FromContext(cmd.Context())
			reg, err := app.Registry(cmd.Context())
			if err != nil {
				return err
			}
			tags, err := reg.ListTags(cmd.Context(), app.Ctx.RepoName(repo))
			if err != nil {
				return err
			}
			// tag desplegado del servicio hermano ({name} compartido); ausente = sin marca
			deployed := ""
			if dep, err := app.Deployer(cmd.Context()); err == nil {
				if cur, err := dep.CurrentTag(cmd.Context(), app.Ctx.ServiceName(repo)); err == nil {
					deployed = cur
				}
			}
			now := time.Now()
			rows := make([][]string, 0, len(tags))
			for _, t := range tags {
				mark := ""
				if deployed != "" && t.Tag == deployed {
					mark = "● now"
				}
				rows = append(rows, []string{t.Tag, render.Age(t.PushedAt, now),
					render.Size(t.SizeBytes), render.ShortDigest(t.Digest), mark})
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), render.Table([]string{"TAG", "AGE", "SIZE", "DIGEST", "DEPLOYED"}, rows))
			return nil
		},
	}
	cmd.Flags().StringVarP(&repo, "repo", "r", "", "repository short name")
	return cmd
}
