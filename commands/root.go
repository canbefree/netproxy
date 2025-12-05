package commands

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "netproxy",
	Short: "A simple and efficient network proxy tool",
	Long:  `NetProxy is a lightweight and high-performance network proxy tool that supports multiple protocols and configurations.`,
}

func Execute() {
	rootCmd.Execute()
}
