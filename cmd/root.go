package cmd

import (
	"errors"
	"fmt"
	"os"

	"quaily-journalist/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	appCfg  config.Config
)

// rootCmd is the base command called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "quaily-journalist",
	Short: "Quaily Journalist CLI",
	Long:  "Minimal CLI using Cobra, Viper, and Redis.",
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./config.yaml)")
}

func initConfig() {
	v := viper.GetViper()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("$HOME/.config/quaily-journalist")
		v.AddConfigPath("configs")
	}

	if err := v.ReadInConfig(); err != nil {
		var nf viper.ConfigFileNotFoundError
		if !errors.As(err, &nf) {
			fmt.Fprintf(os.Stderr, "error reading config: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Using config file: %s\n", v.ConfigFileUsed())
	}

	if err := v.Unmarshal(&appCfg); err != nil {
		fmt.Fprintf(os.Stderr, "error parsing config: %v\n", err)
		os.Exit(1)
	}

	appCfg.FillDefaults()
}

// GetConfig exposes the loaded configuration to subcommands.
func GetConfig() config.Config {
	return appCfg
}
