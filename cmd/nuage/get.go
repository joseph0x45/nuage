package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/joseph0x45/nuage/internal/core"
)

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <id> <dest-path>",
		Short: "Download an indexed file by id (scripting/debugging only; use the web UI day-to-day)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q: %w", args[0], err)
			}

			cfg, idx, err := loadEngineDeps()
			if err != nil {
				return err
			}
			defer idx.Close()

			engine, err := core.New(cfg, idx)
			if err != nil {
				return err
			}
			defer engine.Close()

			rec, err := engine.Download(context.Background(), id, args[1])
			if err != nil {
				return err
			}
			fmt.Printf("Downloaded %s to %s\n", rec.Filename, args[1])
			return nil
		},
	}
}
