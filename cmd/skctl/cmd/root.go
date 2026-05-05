package cmd

import (
	"github.com/spf13/cobra"
)

// Version is overridden via -ldflags at release time.
var Version = "dev"

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "skctl",
		Short:         "exam-run admin & test CLI",
		Long:          "skctl drives exam-run features without going through HTTP, plus a few HTTP probes for tests.",
		SilenceUsage:  true,
		SilenceErrors: false,
		Version:       Version,
	}
	root.PersistentFlags().String("server", "http://localhost:8080", "exam-run server base URL (used by HTTP-based subcommands)")
	root.AddCommand(newHealthCmd())
	root.AddCommand(newVersionCmd())
	return root
}
