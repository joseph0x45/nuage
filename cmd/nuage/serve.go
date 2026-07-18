package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/joseph0x45/nuage/internal/core"
	"github.com/joseph0x45/nuage/internal/web"
)

func newServeCmd() *cobra.Command {
	var addr string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Nuage web server (bind locally; expose via Cloudflare Tunnel)",
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

			httpServer := &http.Server{
				Addr:              addr,
				Handler:           web.NewServer(engine),
				ReadHeaderTimeout: 10 * time.Second,
			}

			fmt.Printf("Nuage listening on %s\n", addr)
			return httpServer.ListenAndServe()
		},
	}

	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8080", "address to bind (localhost/LAN only — expose publicly via cloudflared)")

	return cmd
}
