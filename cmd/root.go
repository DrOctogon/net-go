// root.go viper root command code
package cmd

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/tphakala/voicewatch/cmd/authors"
	"github.com/tphakala/voicewatch/cmd/benchmark"
	"github.com/tphakala/voicewatch/cmd/license"
	"github.com/tphakala/voicewatch/cmd/notify"
	"github.com/tphakala/voicewatch/cmd/serve"
	"github.com/tphakala/voicewatch/cmd/support"
	"github.com/tphakala/voicewatch/internal/conf"
)

// RootCommand creates and returns the root command
func RootCommand(settings *conf.Settings) *cobra.Command {
	// Create the root command
	rootCmd := &cobra.Command{
		Use:   "voicewatch",
		Short: "VoiceWatch CLI",
	}

	// Set up the global flags for the root command.
	err := setupFlags(rootCmd, settings)
	if err != nil {
		log.Printf("error setting up flags: %v\n", err)
	}

	// Add sub-commands to the root command.
	serveCmd := serve.Command(settings)
	authorsCmd := authors.Command()
	licenseCmd := license.Command()
	supportCmd := support.Command(settings)
	benchmarkCmd := benchmark.Command(settings)
	notifyCmd := notify.Command(settings)

	subcommands := []*cobra.Command{
		serveCmd,
		authorsCmd,
		licenseCmd,
		supportCmd,
		benchmarkCmd,
		notifyCmd,
	}

	rootCmd.AddCommand(subcommands...)

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Skip setup for authors and license commands
		if cmd.Name() != authorsCmd.Name() && cmd.Name() != licenseCmd.Name() {
			if err := initialize(); err != nil {
				return fmt.Errorf("error initializing: %w", err)
			}
		}

		return nil
	}

	return rootCmd
}

// initialize is called before any subcommands are run, but after the context is ready
// This function is responsible for setting up configurations, ensuring the environment is ready, etc.
func initialize() error {
	return nil
}

// defineGlobalFlags defines flags that are global to the command line interface
func setupFlags(rootCmd *cobra.Command, settings *conf.Settings) error {
	rootCmd.PersistentFlags().StringVarP(&conf.ConfigPath, "config", "c", conf.ConfigPath, "Path to config file (defaults to OS-specific config search locations)")
	rootCmd.PersistentFlags().BoolVarP(&settings.Debug, "debug", "d", viper.GetBool("debug"), "Enable debug output")
	rootCmd.PersistentFlags().StringVar(&settings.VoiceWatch.Locale, "locale", viper.GetString("voicewatch.locale"), "Set the locale for labels. Accepts full name or 2-letter code.")
	rootCmd.PersistentFlags().IntVarP(&settings.VoiceWatch.Threads, "threads", "j", viper.GetInt("voicewatch.threads"), "Number of CPU threads to use for analysis (default 0 which is all CPUs)")
	rootCmd.PersistentFlags().Float64VarP(&settings.VoiceWatch.Sensitivity, "sensitivity", "s", viper.GetFloat64("voicewatch.sensitivity"), "Sigmoid sensitivity value between 0.0 and 1.5")
	rootCmd.PersistentFlags().Float64VarP(&settings.VoiceWatch.Threshold, "threshold", "t", viper.GetFloat64("voicewatch.threshold"), "Confidency threshold for detections, value between 0.1 to 1.0")
	rootCmd.PersistentFlags().Float64Var(&settings.VoiceWatch.Overlap, "overlap", viper.GetFloat64("voicewatch.overlap"), "Overlap value between 0.0 and 2.9")
	rootCmd.PersistentFlags().Float64Var(&settings.VoiceWatch.Latitude, "latitude", viper.GetFloat64("voicewatch.latitude"), "Latitude for species prediction")
	rootCmd.PersistentFlags().Float64Var(&settings.VoiceWatch.Longitude, "longitude", viper.GetFloat64("voicewatch.longitude"), "Longitude for species prediction")

	// Bind flags to the viper settings
	if err := viper.BindPFlags(rootCmd.PersistentFlags()); err != nil {
		return fmt.Errorf("error binding flags: %w", err)
	}

	return nil
}
