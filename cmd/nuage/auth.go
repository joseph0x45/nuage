package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/joseph0x45/nuage/internal/config"
	nuagetg "github.com/joseph0x45/nuage/internal/telegram"
)

func newAuthCmd() *cobra.Command {
	var apiID int
	var apiHash string

	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Log in to Telegram (phone/code/2FA) and persist the session",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if errors.Is(err, os.ErrNotExist) {
				cfg = &config.Config{}
			} else if err != nil {
				return err
			}

			if apiID != 0 {
				cfg.ApiID = apiID
			}
			if apiHash != "" {
				cfg.ApiHash = apiHash
			}
			if cfg.ApiID == 0 {
				id, err := promptInt("API ID (from my.telegram.org): ")
				if err != nil {
					return err
				}
				cfg.ApiID = id
			}
			if cfg.ApiHash == "" {
				hash, err := promptString("API hash (from my.telegram.org): ")
				if err != nil {
					return err
				}
				cfg.ApiHash = hash
			}

			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			client, err := nuagetg.NewClient(cfg)
			if err != nil {
				return err
			}

			return nuagetg.Login(context.Background(), client)
		},
	}

	cmd.Flags().IntVar(&apiID, "api-id", 0, "Telegram API ID (from my.telegram.org)")
	cmd.Flags().StringVar(&apiHash, "api-hash", "", "Telegram API hash (from my.telegram.org)")

	return cmd
}

func promptString(label string) (string, error) {
	fmt.Print(label)
	var s string
	if _, err := fmt.Scanln(&s); err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(s), nil
}

func promptInt(label string) (int, error) {
	s, err := promptString(label)
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("expected a number, got %q", s)
	}
	return n, nil
}
