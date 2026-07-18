package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/joseph0x45/nuage/internal/config"
	nuagetg "github.com/joseph0x45/nuage/internal/telegram"
)

func newInitCmd() *cobra.Command {
	var title string
	var about string
	var useExisting bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create or select the private Telegram channel Nuage stores files in",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("no config found; run `nuage auth` first")
			} else if err != nil {
				return err
			}
			if cfg.ApiID == 0 || cfg.ApiHash == "" {
				return fmt.Errorf("missing API credentials; run `nuage auth` first")
			}

			client, err := nuagetg.NewClient(cfg)
			if err != nil {
				return err
			}

			return client.Run(context.Background(), func(ctx context.Context) error {
				status, err := client.Auth().Status(ctx)
				if err != nil {
					return fmt.Errorf("check auth status: %w", err)
				}
				if !status.Authorized {
					return fmt.Errorf("not logged in; run `nuage auth` first")
				}

				api := client.API()

				var channelID, accessHash int64
				var channelTitle string

				if useExisting {
					channels, err := nuagetg.AdminChannels(ctx, api)
					if err != nil {
						return err
					}
					if len(channels) == 0 {
						return fmt.Errorf("no channels found where this account is an admin/creator")
					}
					fmt.Println("Channels you administer:")
					for i, ch := range channels {
						fmt.Printf("  %d) %s\n", i+1, ch.Title)
					}
					choice, err := promptInt(fmt.Sprintf("Pick a channel [1-%d]: ", len(channels)))
					if err != nil {
						return err
					}
					if choice < 1 || choice > len(channels) {
						return fmt.Errorf("invalid choice %d", choice)
					}
					selected := channels[choice-1]
					channelID = selected.ID
					accessHash = selected.AccessHash
					channelTitle = selected.Title
				} else {
					if title == "" {
						title, err = promptString("Channel title (e.g. \"Nuage Storage\"): ")
						if err != nil {
							return err
						}
					}
					ch, err := nuagetg.CreateChannel(ctx, api, title, about)
					if err != nil {
						return err
					}
					channelID = ch.ID
					accessHash = ch.AccessHash
					channelTitle = ch.Title
				}

				cfg.ChannelID = channelID
				cfg.ChannelAccessHash = accessHash
				if err := config.Save(cfg); err != nil {
					return fmt.Errorf("save config: %w", err)
				}

				fmt.Printf("Storage channel set: %q (id=%d)\n", channelTitle, channelID)
				return nil
			})
		},
	}

	cmd.Flags().StringVar(&title, "title", "", "Title for the new storage channel")
	cmd.Flags().StringVar(&about, "about", "", "Description for the new storage channel")
	cmd.Flags().BoolVar(&useExisting, "existing", false, "Pick an existing channel you administer instead of creating one")

	return cmd
}
