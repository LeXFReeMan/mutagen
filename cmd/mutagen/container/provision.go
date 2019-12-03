package container

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mutagen-io/mutagen/pkg/mutagen"
)

func provisionMain(command *cobra.Command, arguments []string) error {
	// TODO: Implement.
	fmt.Println("Provisioning for Mutagen", mutagen.Version)

	// Success.
	return nil
}

var provisionCommand = &cobra.Command{
	Use:          "provision",
	Short:        "Provision a container for usage with Mutagen",
	RunE:         provisionMain,
	SilenceUsage: true,
}

var provisionConfiguration struct {
	// help indicates whether or not help information should be shown for the
	// command.
	help bool
}

func init() {
	// Grab a handle for the command line flags.
	flags := provisionCommand.Flags()

	// Disable alphabetical sorting of flags in help output.
	flags.SortFlags = false

	// Manually add a help flag to override the default message. Cobra will
	// still implement its logic automatically.
	flags.BoolVarP(&provisionConfiguration.help, "help", "h", false, "Show help information")
}
