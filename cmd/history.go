package cmd

import (
	"fmt"
	"os"

	"tai/internal/history"
	"tai/internal/tui"

	"github.com/spf13/cobra"
)

// historyYes flips the post-selection action from "copy to clipboard" (default)
// to "execute via bash -c", matching the semantics of -y on the root command.
var historyYes bool

// Injection points for tests.
var (
	getHistoryEntries = history.GetEntries
	runHistoryTUI     = tui.RunHistory
)

var historyCmd = &cobra.Command{
	Use:     "history",
	Aliases: []string{"h"},
	Short:   "Browse previously generated commands and re-use one",
	Long: "Open a fuzzy-searchable list of past tai invocations. " +
		"Selecting an entry copies its command to the clipboard, or, with -y, runs it directly.",
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if code := runHistory(); code != 0 {
			os.Exit(code)
		}
	},
}

func runHistory() int {
	entries, err := getHistoryEntries()
	if err != nil {
		fmt.Printf("❌ Failed to load history: %v\n", err)
		return 1
	}
	if len(entries) == 0 {
		fmt.Println("📭 No history yet — run a command first with `tai \"...\"`.")
		return 0
	}

	selected, err := runHistoryTUI(entries)
	if err != nil {
		fmt.Printf("❌ TUI error: %v\n", err)
		return 1
	}
	if selected == nil {
		fmt.Println("Cancelled.")
		return 0
	}

	if historyYes {
		fmt.Printf("\n👉 Selected command:\n\033[1;32m%s\033[0m\n\n", selected.Command)
		executeCommand(selected.Command)
		return 0
	}

	if err := copyCommandToClipboard(selected.Command); err != nil {
		fmt.Printf("❌ Failed to copy to clipboard: %v\n", err)
		return 1
	}
	fmt.Printf("📋 Copied to clipboard: %s\n", selected.Command)
	return 0
}

func init() {
	historyCmd.Flags().BoolVarP(&historyYes, "yes", "y", false, "Run the selected command immediately instead of copying it")
	rootCmd.AddCommand(historyCmd)
}
