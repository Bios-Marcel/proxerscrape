package main

import (
	"log"

	"github.com/spf13/cobra"
)

var verbose = new(bool)

func main() {
	rootCmd := cobra.Command{Use: "proxercli"}
	rootCmd.PersistentFlags().BoolVarP(verbose, "verbose", "v", false, "Decides whether additional, potentially unnecessary extra information, is printed to the terminal.")
	rootCmd.AddCommand(generateCacheCmd())
	if err := rootCmd.Execute(); err != nil {
		log.Fatalln("Error executing root cmd:", err)
	}
}

func generateCacheCmd() *cobra.Command {
	cacheCmd := &cobra.Command{
		Use:     "cache",
		Short:   "TODO",
		Example: "cache clear",
	}
	cacheCmd.AddCommand(&cobra.Command{
		Use:     "info",
		Short:   "TODO",
		Example: "cache info",
		Run: func(cmd *cobra.Command, args []string) {
			log.Println("cache info (DUMMY CALL)")
		},
	})
	cacheCmd.AddCommand(&cobra.Command{
		Use:     "clear",
		Short:   "TODO",
		Example: "cache clear",
		Run: func(cmd *cobra.Command, args []string) {
			log.Println("clearing cache (DUMMY CALL)")
		},
	})

	return cacheCmd
}
