// Package telegram wraps gotd/td client setup and the interactive login flow.
package telegram

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"golang.org/x/term"
)

// terminalAuth implements auth.UserAuthenticator by prompting on stdin/stdout.
// It's only ever used by `nuage auth`; the web server and other commands run
// against an already-persisted session and never construct this.
type terminalAuth struct{}

func (terminalAuth) Phone(_ context.Context) (string, error) {
	return prompt("Phone number (international format, e.g. +15551234567): ")
}

func (terminalAuth) Password(_ context.Context) (string, error) {
	fmt.Print("2FA password: ")
	pw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(pw), nil
}

func (terminalAuth) AcceptTermsOfService(_ context.Context, tos tg.HelpTermsOfService) error {
	fmt.Println(tos.Text)
	return nil
}

func (terminalAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("account not registered on Telegram; sign up in the Telegram app first")
}

func (terminalAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	return prompt("Login code (sent via Telegram): ")
}

func prompt(label string) (string, error) {
	fmt.Print(label)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read input: %w", err)
	}
	return strings.TrimSpace(line), nil
}

// Login runs the interactive phone/code/2FA flow against client and persists
// the resulting session via the client's configured SessionStorage. It's a
// no-op (beyond confirming auth) if the session file already has a valid login.
func Login(ctx context.Context, client *telegram.Client) error {
	return client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("check auth status: %w", err)
		}
		if status.Authorized {
			fmt.Println("Already logged in.")
			return nil
		}

		flow := auth.NewFlow(terminalAuth{}, auth.SendCodeOptions{})
		if err := flow.Run(ctx, client.Auth()); err != nil {
			return fmt.Errorf("auth flow: %w", err)
		}

		self, err := client.Self(ctx)
		if err != nil {
			return fmt.Errorf("fetch self after login: %w", err)
		}
		fmt.Printf("Logged in as %s (@%s).\n", strings.TrimSpace(self.FirstName+" "+self.LastName), self.Username)
		return nil
	})
}
