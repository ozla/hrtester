package mock

import (
	"github.com/ozla/hrtester/internal/config"
	"github.com/ozla/hrtester/internal/log"
	"github.com/ozla/hrtester/internal/mock"
	"github.com/spf13/cobra"
)

////////////////////////////////////////////////////////////////////////////////

var (
	Cmd = &cobra.Command{
		Use:   "mock",
		Short: "Run hrtester in mock mode to simulate target server responses.",
		Run: func(cmd *cobra.Command, args []string) {
			service := mock.NewService()
			if service == nil {
				log.Fatal("Failed to initialize mock service!", nil)
			}
			service.Start()
		},
	}
)

////////////////////////////////////////////////////////////////////////////////

func init() {
	Cmd.Flags().SortFlags = false

	Cmd.Flags().StringVar(
		&config.Mocker.Cert,
		"cert",
		"",
		"Path to the mocker's certificate (PEM file).",
	)
	Cmd.Flags().StringVar(
		&config.Mocker.Key,
		"key",
		"",
		"Path to the mocker's private key (PEM file).",
	)
	Cmd.Flags().StringVar(
		&config.Mocker.CAs,
		"cas",
		"",
		"Path to the trusted CA certificate bundle (PEM file).",
	)
	Cmd.Flags().Uint16Var(
		&config.Mocker.Port,
		"port",
		config.DefaultPort,
		"Port on which hrtester in mock mode will listen.",
	)
}

////////////////////////////////////////////////////////////////////////////////
