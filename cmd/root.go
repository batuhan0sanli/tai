package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"tai/internal/provider"

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

		// Print the command in green (simple ANSI for now, instead of Lipgloss).
		fmt.Printf("\n👉 Suggested command:\n\033[1;32m%s\033[0m\n\n", suggestedCmd)

		// If --copy / -c is set, copy the command and exit without executing.
		if copyToClipboard {
			if err := copyCommandToClipboard(suggestedCmd); err != nil {
				fmt.Printf("❌ Failed to copy to clipboard: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("📋 Command copied to clipboard.")
			return
		}

		// If --yes / -y flag is set, execute directly.
		if skipPermission {
			executeCommand(suggestedCmd)
			return
		}

		// Plain terminal confirmation for now (Phase 2 will replace this with a Bubble Tea TUI).
		fmt.Print("Do you want to run this command? [y/N]: ")
		var input string
		fmt.Scanln(&input)
		input = strings.ToLower(strings.TrimSpace(input))

		if input == "y" || input == "yes" {
			executeCommand(suggestedCmd)
		} else {
			fmt.Println("Cancelled.")
		}
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
