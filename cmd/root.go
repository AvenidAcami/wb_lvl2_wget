package cmd

import (
	"fmt"
	"log"
	"os"
	"time"
	"wb_lvl2_wget/internal/downloader"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
	Use:   "mirror [url]",
	Short: "wget",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		url := args[0]
		out := viper.GetString("out")
		depth := viper.GetInt("depth")
		timeout := viper.GetDuration("timeout")

		if out == "" {
			out = "mirror_output"
		}

		cfg := downloader.Config{
			OutDir:   out,
			MaxDepth: depth,
			Timeout:  timeout,
		}

		d, err := downloader.New(cfg)
		if err != nil {
			log.Fatalf("failed to create downloader: %v", err)
		}

		start := time.Now()
		if err := d.Run(url); err != nil {
			log.Printf("Error: %v", err)
		}
		fmt.Printf("Finished in %v\n", time.Since(start))
	},
}

func init() {
	rootCmd.PersistentFlags().Int("depth", 2, "recursion depth")
	rootCmd.PersistentFlags().String("out", "mirror_output", "output directory")
	rootCmd.PersistentFlags().Duration("timeout", 15*time.Second, "HTTP timeout")

	viper.BindPFlag("depth", rootCmd.PersistentFlags().Lookup("depth"))
	viper.BindPFlag("out", rootCmd.PersistentFlags().Lookup("out"))
	viper.BindPFlag("timeout", rootCmd.PersistentFlags().Lookup("timeout"))
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
