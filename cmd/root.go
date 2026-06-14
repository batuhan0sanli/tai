package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"tai/internal/provider"
	"tai/internal/tui"

	"github.com/spf13/cobra"
)

var (
	skipPermission  bool
	copyToClipboard bool
)

var rootCmd = &cobra.Command{
	Use:   "tai \"[request]\"",
	Short: "A lightweight AI assistant for terminal operations.",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		userPrompt := strings.Join(args, " ")

		fmt.Println("🤖 tai is thinking...")

		// For now we use the Claude CLI provider as the default.
		ai := provider.NewClaudeCLIProvider()
		suggestedCmd, err := ai.GenerateCommand(userPrompt)
		if err != nil {
			fmt.Printf("❌ Error while invoking Claude: %v\n", err)
			os.Exit(1)
		}

		// --copy / -c: dump to clipboard and bail before the TUI.
		if copyToClipboard {
			fmt.Printf("\n👉 Suggested command:\n\033[1;32m%s\033[0m\n\n", suggestedCmd)
			if err := copyCommandToClipboard(suggestedCmd); err != nil {
				fmt.Printf("❌ Failed to copy to clipboard: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("📋 Command copied to clipboard.")
			return
		}

		// --yes / -y: run immediately, skip the TUI entirely.
		if skipPermission {
			fmt.Printf("\n👉 Suggested command:\n\033[1;32m%s\033[0m\n\n", suggestedCmd)
			executeCommand(suggestedCmd)
			return
		}

		// Default path: hand off to the Bubble Tea TUI for review / revision.
		finalCmd, shouldRun, err := tui.Run(userPrompt, suggestedCmd, ai)
		if err != nil {
			fmt.Printf("❌ TUI error: %v\n", err)
			os.Exit(1)
		}
		if !shouldRun {
			fmt.Println("Cancelled.")
			return
		}
		executeCommand(finalCmd)
	},
}

func executeCommand(shellCmd string) {
	fmt.Println("Running command...")
	cmd := exec.Command("bash", "-c", shellCmd) // May need adaptation for Windows ("cmd /c" or "powershell -c").
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		fmt.Printf("\n❌ Command exited with error: %v\n", err)
	}
}

func copyCommandToClipboard(text string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	case "linux":
		cmd = exec.Command("xclip", "-selection", "clipboard")
	case "windows":
		cmd = exec.Command("clip")
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVarP(&skipPermission, "yes", "y", false, "Skip the confirmation prompt and run the command directly")
	rootCmd.Flags().BoolVarP(&copyToClipboard, "copy", "c", false, "Do not run the command, only copy it to the clipboard")
}
