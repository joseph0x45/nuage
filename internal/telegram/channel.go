package telegram

import (
	"context"
	"fmt"

	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/telegram/query/dialogs"
	"github.com/gotd/td/tg"
)

// CreateChannel creates a new private broadcast channel (Nuage's storage
// target is always broadcast, not a megagroup/supergroup chat) and returns
// the resulting tg.Channel, which carries the ID/AccessHash pair Nuage needs
// to address it later.
func CreateChannel(ctx context.Context, api *tg.Client, title, about string) (*tg.Channel, error) {
	updates, err := api.ChannelsCreateChannel(ctx, &tg.ChannelsCreateChannelRequest{
		Broadcast: true,
		Title:     title,
		About:     about,
	})
	if err != nil {
		return nil, fmt.Errorf("create channel: %w", err)
	}

	u, ok := updates.(*tg.Updates)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T from channel creation", updates)
	}
	for _, chat := range u.Chats {
		if ch, ok := chat.(*tg.Channel); ok {
			return ch, nil
		}
	}
	return nil, fmt.Errorf("channel creation response did not include the new channel")
}

// AdminChannels returns the channels the logged-in account created or
// administers, for `nuage init --existing` to let the user attach Nuage to a
// channel they already made instead of creating a new one.
func AdminChannels(ctx context.Context, api *tg.Client) ([]*tg.Channel, error) {
	var channels []*tg.Channel
	seen := make(map[int64]bool)

	err := query.GetDialogs(api).ForEach(ctx, func(ctx context.Context, elem dialogs.Elem) error {
		for id, ch := range elem.Entities.Channels() {
			if seen[id] {
				continue
			}
			if ch.Creator || ch.AdminRights.PostMessages {
				seen[id] = true
				channels = append(channels, ch)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list dialogs: %w", err)
	}
	return channels, nil
}
