package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/joseph0x45/nuage/internal/auth"
	"github.com/joseph0x45/nuage/internal/config"
)

func newPasswordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "password",
		Short: "Set or update the password required to log in to the web UI",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("no config found; run `nuage auth` first")
			} else if err != nil {
				return err
			}

			password, err := promptPassword("New web UI password: ")
			if err != nil {
				return err
			}
			if len(password) < 8 {
				return fmt.Errorf("password must be at least 8 characters")
			}
			confirm, err := promptPassword("Confirm password: ")
			if err != nil {
				return err
			}
			if password != confirm {
				return fmt.Errorf("passwords did not match")
			}

			hash, err := auth.HashPassword(password)
			if err != nil {
				return err
			}
			cfg.WebPasswordHash = hash

			if cfg.SessionSecret == "" {
				secret, err := auth.GenerateSecret()
				if err != nil {
					return err
				}
				cfg.SessionSecret = secret
			}

			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}

			fmt.Println("Web UI password set.")
			return nil
		},
	}
}

func promptPassword(label string) (string, error) {
	fmt.Print(label)
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(pw), nil
}
