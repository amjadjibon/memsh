package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of memsh",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("memsh v0.1.0")
	},
}
