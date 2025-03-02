package cmd

import (
	"github.com/ozla/hrtester/cmd/collector"
	"github.com/ozla/hrtester/cmd/mock"
	"github.com/ozla/hrtester/cmd/tester"
	"github.com/ozla/hrtester/internal/log"
	"github.com/spf13/cobra"
)

////////////////////////////////////////////////////////////////////////////////

var (
	rootCmd = &cobra.Command{
		Use:   "hrtester",
		Short: "hrtester is an HTTP roundtrip benchmarking tool",
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
	Execute = rootCmd.Execute
)

////////////////////////////////////////////////////////////////////////////////

func init() {
	cobra.OnInitialize(log.Init)

	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.Flags().SortFlags = false

	rootCmd.PersistentFlags().StringVar(
		&log.FileName,
		"log",
		"",
		"Path to the log file.",
	)
	rootCmd.PersistentFlags().BoolVar(
		&log.Debugging,
		"debug",
		false,
		"Enable debug mode for verbose logging.",
	)

	rootCmd.AddCommand(tester.Cmd, collector.Cmd, mock.Cmd)
}

////////////////////////////////////////////////////////////////////////////////
