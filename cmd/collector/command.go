package collector

import (
	"os"

	"github.com/ozla/hrtester/internal/collector"
	"github.com/ozla/hrtester/internal/config"
	"github.com/ozla/hrtester/internal/log"
	"github.com/spf13/cobra"
)

////////////////////////////////////////////////////////////////////////////////

var (
	Cmd = &cobra.Command{
		Use:   "collect",
		Short: "Run hrtester in collector mode.",
		Run: func(cmd *cobra.Command, args []string) {
			service := collector.NewCollectService()
			if service == nil {
				log.Fatal("failed to initialize collector service", nil)
			}
			service.Start()
		},
	}
)

////////////////////////////////////////////////////////////////////////////////

func init() {
	Cmd.Flags().SortFlags = false

	Cmd.Flags().StringVar(
		&config.Collector.CSVFile,
		"csv",
		"",
		"Path to a CSV file for test results. (required)",
	)
	Cmd.Flags().Uint16Var(
		&config.Collector.Port,
		"port",
		config.DefaultPort,
		"Port on which hrtester in collector mode will listen.",
	)
	if err := Cmd.MarkFlagRequired("csv"); err != nil {
		os.Exit(1)
	}
}

////////////////////////////////////////////////////////////////////////////////
