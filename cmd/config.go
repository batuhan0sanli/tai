package cmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"

	"tai/internal/config"

	"github.com/spf13/cobra"
)

var configForce bool

// Injection points for the config subcommand, overridden in tests.
var (
	configFilePath = config.FilePath
	configSave     = config.Save
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage tai's provider configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write a starter config (all providers stubbed) to ~/.config/tai/config.json",
	Run: func(cmd *cobra.Command, args []string) {
		if code := runConfigInit(); code != 0 {
			os.Exit(code)
		}
	},
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the path to the config file",
	Run: func(cmd *cobra.Command, args []string) {
		if code := runConfigPath(); code != 0 {
			os.Exit(code)
		}
	},
}

// runConfigInit writes the config template, refusing to clobber an existing
// file unless --force is set. Returns the process exit code.
func runConfigInit() int {
	path, err := configFilePath()
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return 1
	}
	switch _, statErr := os.Stat(path); {
	case statErr == nil:
		if !configForce {
			fmt.Printf("⚠️  config already exists at %s (use --force to overwrite)\n", path)
			return 1
		}
	case !errors.Is(statErr, fs.ErrNotExist):
		// A permission error / broken symlink / ENOTDIR shouldn't be silently
		// treated as "doesn't exist" and then overwritten.
		fmt.Printf("❌ cannot check config path %s: %v\n", path, statErr)
		return 1
	}
	if err := configSave(config.Template()); err != nil {
		fmt.Printf("❌ failed to write config: %v\n", err)
		return 1
	}
	fmt.Printf("✅ wrote config template to %s\n", path)
	fmt.Println("   Fill in API keys / models and set \"default_provider\" to the one you want.")
	return 0
}

// runConfigPath prints the config file path. Returns the process exit code.
func runConfigPath() int {
	path, err := configFilePath()
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return 1
	}
	fmt.Println(path)
	return 0
}

func init() {
	configInitCmd.Flags().BoolVar(&configForce, "force", false, "Overwrite an existing config file")
	configCmd.AddCommand(configInitCmd, configPathCmd)
	rootCmd.AddCommand(configCmd)
}
