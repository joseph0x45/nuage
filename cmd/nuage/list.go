package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/joseph0x45/nuage/internal/core"
)

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List indexed files (scripting/debugging only; use the web UI day-to-day)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
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

			records, err := engine.List(context.Background())
			if err != nil {
				return err
			}
			if len(records) == 0 {
				fmt.Println("No files indexed yet.")
				return nil
			}
			for _, rec := range records {
				fmt.Printf("%d\t%s\t%d bytes\t%s\n", rec.ID, rec.Filename, rec.Size, rec.UploadedAt.Format("2006-01-02 15:04"))
			}
			return nil
		},
	}
}
