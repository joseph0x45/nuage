package telegram

import (
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"

	"github.com/joseph0x45/nuage/internal/config"
)

// NewClient builds an unstarted gotd client using cfg's API credentials and
// the standard Nuage session file location. Callers must invoke client.Run
// (directly, or via Login) before making any API calls.
func NewClient(cfg *config.Config) (*telegram.Client, error) {
	sessionPath, err := config.SessionPath()
	if err != nil {
		return nil, err
	}
	client := telegram.NewClient(cfg.ApiID, cfg.ApiHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: sessionPath},
	})
	return client, nil
}
