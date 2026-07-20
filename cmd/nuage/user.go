package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/joseph0x45/nuage/internal/auth"
	"github.com/joseph0x45/nuage/internal/config"
	"github.com/joseph0x45/nuage/internal/index"
)

func newUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Manage web UI login profiles",
	}
	cmd.AddCommand(newUserAddCmd())
	cmd.AddCommand(newUserListCmd())
	cmd.AddCommand(newUserRmCmd())
	return cmd
}

func newUserAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <username>",
		Short: "Create a profile, or update its password if it already exists",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]

			cfg, err := config.Load()
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("no config found; run `nuage auth` first")
			} else if err != nil {
				return err
			}

			password, err := promptPassword(fmt.Sprintf("Password for %s: ", username))
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

			// The very first profile created inherits every file uploaded
			// before profiles existed (via the old shared-password login or
			// the unscoped CLI commands).
			firstProfile := len(cfg.Users) == 0
			isNew := cfg.UpsertUser(username, hash)

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

			if firstProfile {
				indexPath, err := config.IndexPath()
				if err != nil {
					return err
				}
				idx, err := index.Open(indexPath)
				if err != nil {
					return fmt.Errorf("open index to assign existing files: %w", err)
				}
				defer idx.Close()
				if err := idx.BackfillOwner(context.Background(), username); err != nil {
					return fmt.Errorf("assign existing files to %s: %w", username, err)
				}
				fmt.Printf("Profile %q created; any previously indexed files are now assigned to it.\n", username)
				return nil
			}

			if isNew {
				fmt.Printf("Profile %q created.\n", username)
			} else {
				fmt.Printf("Password updated for %q.\n", username)
			}
			return nil
		},
	}
}

func newUserListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured web UI profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("no config found; run `nuage auth` first")
			} else if err != nil {
				return err
			}
			if len(cfg.Users) == 0 {
				fmt.Println("No profiles configured yet. Run `nuage user add <username>`.")
				return nil
			}
			for _, u := range cfg.Users {
				fmt.Println(u.Username)
			}
			return nil
		},
	}
}

func newUserRmCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rm <username>",
		Short: "Remove a profile's web UI access (its files stay indexed under that name)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			username := args[0]

			cfg, err := config.Load()
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("no config found; run `nuage auth` first")
			} else if err != nil {
				return err
			}

			if len(cfg.Users) <= 1 {
				return fmt.Errorf("refusing to remove the last profile; add another with `nuage user add` first")
			}
			if !cfg.RemoveUser(username) {
				return fmt.Errorf("no profile named %q", username)
			}

			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("save config: %w", err)
			}
			fmt.Printf("Removed profile %q. Its files remain indexed under that name.\n", username)
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
