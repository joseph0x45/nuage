package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/joseph0x45/nuage/internal/config"
	"github.com/joseph0x45/nuage/internal/core"
	"github.com/joseph0x45/nuage/internal/index"
)

func newUploadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "upload <path>",
		Short: "Upload a file to the storage channel (scripting/debugging only, not tied to a profile; use the web UI day-to-day)",
		Args:  cobra.ExactArgs(1),
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

			rec, err := engine.Upload(context.Background(), args[0], filepath.Base(args[0]), "")
			if err != nil {
				return err
			}
			fmt.Printf("Uploaded %s (id=%d, hash=%s, size=%d bytes, message_id=%d)\n",
				rec.Filename, rec.ID, rec.Hash, rec.Size, rec.MessageID)
			return nil
		},
	}
}

func loadEngineDeps() (*config.Config, *index.Index, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load config (run `nuage auth` and `nuage init` first): %w", err)
	}
	indexPath, err := config.IndexPath()
	if err != nil {
		return nil, nil, err
	}
	idx, err := index.Open(indexPath)
	if err != nil {
		return nil, nil, err
	}
	return cfg, idx, nil
}
