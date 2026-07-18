// Command nuage is the setup CLI for Nuage: Telegram login, storage channel
// configuration, and launching the web server. Day-to-day file operations
// happen through the web UI, not here.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "nuage",
		Short: "Personal cloud storage backed by a private Telegram channel",
	}

	root.AddCommand(newAuthCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newUploadCmd())
	root.AddCommand(newGetCmd())
	root.AddCommand(newListCmd())
	root.AddCommand(newServeCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
