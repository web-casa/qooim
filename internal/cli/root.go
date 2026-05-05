package cli

import (
	"github.com/spf13/cobra"
)

// Version is overridden via -ldflags at release time.
var Version = "dev"

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "qooim",
		Short:         "Qoo.IM admin & test CLI",
		Long:          "qooim drives Qoo.IM features without going through HTTP, plus a few HTTP probes for tests.",
		SilenceUsage:  true,
		SilenceErrors: false,
		Version:       Version,
	}
	root.PersistentFlags().String("server", "http://localhost:8080", "Qoo.IM server base URL (used by HTTP-based subcommands)")
	root.AddCommand(newHealthCmd())
	root.AddCommand(newVersionCmd())
	root.AddCommand(newLoginCmd())
	root.AddCommand(newLogoutCmd())
	root.AddCommand(newMeCmd())
	for _, c := range newListGroup() {
		root.AddCommand(c)
	}
	return root
}
