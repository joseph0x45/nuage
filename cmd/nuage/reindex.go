package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/joseph0x45/nuage/internal/core"
)

func newReindexCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reindex",
		Short: "Rebuild the local index from scratch by scanning the storage channel",
		Long: "Wipes the local index and rebuilds it by walking every message in the storage\n" +
			"channel and parsing the metadata (path, filename, owner, hash) each upload's\n" +
			"caption carries. Use this after losing index.db, or when moving to a new\n" +
			"machine. Messages without recognizable Nuage metadata are skipped — files\n" +
			"uploaded before this feature existed need `nuage backfill-captions` first.",
		Args: cobra.NoArgs,
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

			fmt.Println("Scanning storage channel...")
			count, err := engine.Reindex(context.Background())
			if err != nil {
				return err
			}
			fmt.Printf("Reindexed %d file(s).\n", count)
			return nil
		},
	}
}

func newBackfillCaptionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "backfill-captions",
		Short: "Write disaster-recovery metadata into the caption of every already-indexed file",
		Long: "New uploads already carry their own recovery metadata (see `nuage reindex`).\n" +
			"This is a one-time step for files uploaded before that existed, so `nuage\n" +
			"reindex` can recover them too. Safe to re-run.",
		Args: cobra.NoArgs,
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

			count, err := engine.BackfillCaptions(context.Background())
			if err != nil {
				fmt.Printf("Updated %d file(s) before failing.\n", count)
				return err
			}
			fmt.Printf("Updated %d file(s).\n", count)
			return nil
		},
	}
}
