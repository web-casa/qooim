package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

// newListGroup builds the `qooim {project,repo,template,dashboard} list` cobra tree.
// All four share the same paginated GET shape, so they're emitted from a loop.
func newListGroup() []*cobra.Command {
	specs := []struct{ noun, path string }{
		{"project", "/api/projects"},
		{"repo", "/api/repos"},
		{"template", "/api/templates"},
		{"dashboard", "/api/dashboards"},
	}
	out := make([]*cobra.Command, 0, len(specs))
	for _, sp := range specs {
		group := &cobra.Command{
			Use:   sp.noun,
			Short: fmt.Sprintf("Operate on %ss", sp.noun),
		}
		group.AddCommand(newListCmd(sp.noun, sp.path))
		out = append(out, group)
	}
	return out
}

func newListCmd(noun, path string) *cobra.Command {
	var page, pageSize int
	cmd := &cobra.Command{
		Use:   "list",
		Short: fmt.Sprintf("List %ss (paginated)", noun),
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, _ := cmd.Flags().GetString("server")
			tok := tokenStore{}.load()
			if tok == "" {
				return fmt.Errorf("not logged in; run `qooim login`")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			cli := newClient(base, tok)
			q := url.Values{}
			if page > 0 {
				q.Set("page", strconv.Itoa(page))
			}
			if pageSize > 0 {
				q.Set("page_size", strconv.Itoa(pageSize))
			}
			var res any
			if err := cli.get(ctx, path, q, &res); err != nil {
				return err
			}
			b, _ := json.MarshalIndent(res, "", "  ")
			fmt.Println(string(b))
			return nil
		},
	}
	cmd.Flags().IntVar(&page, "page", 0, "page number (1-based; default 1)")
	cmd.Flags().IntVar(&pageSize, "page-size", 0, "page size (default 20, max 200)")
	return cmd
}
