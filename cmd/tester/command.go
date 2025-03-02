package tester

import (
	"os"

	"github.com/ozla/hrtester/internal/config"
	"github.com/ozla/hrtester/internal/log"
	"github.com/ozla/hrtester/internal/tester"
	"github.com/spf13/cobra"
)

////////////////////////////////////////////////////////////////////////////////

var (
	Cmd = &cobra.Command{
		Use:   "test",
		Short: "Run hrtester in test mode.",
		Run: func(cmd *cobra.Command, args []string) {
			service := tester.NewService()
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
		&config.Tester.Target,
		"target",
		"",
		"Target IP and port to benchmark. (required)",
	)
	Cmd.Flags().StringVar(
		&config.Tester.Collector,
		"collector",
		"",
		"Collector IP and port. (required)",
	)
	Cmd.Flags().StringVar(
		&config.Tester.CAs,
		"cas",
		"",
		"Path to the trusted CA certificate bundle (PEM file).",
	)
	Cmd.Flags().BoolVar(
		&config.Tester.SkipNameCheck,
		"skip-name-check",
		false,
		"Skip target name verification for HTTPS requests",
	)
	Cmd.Flags().StringVar(
		&config.Tester.Cert,
		"cert",
		"",
		"Path to the tester’s client certificate (PEM file).",
	)
	Cmd.Flags().StringVar(
		&config.Tester.Key,
		"key",
		"",
		"Path to the tester’s private key (PEM file).",
	)
	Cmd.Flags().Uint16Var(
		&config.Tester.Port,
		"port",
		config.DefaultPort,
		"Port on which hrtester in test mode will listen.",
	)
	if err := Cmd.MarkFlagRequired("collector"); err != nil {
		os.Exit(1)
	}
	if err := Cmd.MarkFlagRequired("target"); err != nil {
		os.Exit(1)
	}
}

////////////////////////////////////////////////////////////////////////////////
