package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"tai/internal/history"
	"tai/internal/provider"
	"tai/internal/tui"

	"github.com/spf13/cobra"
)

var (
	skipPermission  bool
	copyToClipboard bool
	noTUI           bool
)

// Build metadata. These are overridden at link time by goreleaser via
// -ldflags "-X tai/cmd.version=... -X tai/cmd.commit=... -X tai/cmd.date=...".
// The defaults below are what `go build` / `go run` produce locally.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// versionString is the body of `tai --version`. Kept as a function so the
// formatting can be unit-tested without depending on cobra's template engine.
func versionString() string {
	return fmt.Sprintf("tai version %s\ncommit:  %s\nbuilt:   %s\n", version, commit, date)
}

// Injection points: overridden by tests so the root command can be exercised
// without the live `claude` CLI or a real TTY.
var (
	newProvider           = func() provider.AIProvider { return provider.NewClaudeCLIProvider() }
	runTUI                = tui.Run
	saveHistory           = history.SaveEntry
	stdin       io.Reader = os.Stdin
)

var rootCmd = &cobra.Command{
	Use:   "tai \"[request]\"",
	Short: "A lightweight AI assistant for terminal operations.",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if code := runRoot(args); code != 0 {
			os.Exit(code)
		}
	},
}

// runRoot is the testable body of the root command. It returns the process
// exit code rather than calling os.Exit directly so tests can assert on it.
func runRoot(args []string) int {
	userPrompt := strings.Join(args, " ")

	fmt.Println("🤖 tai is thinking...")

	ai := newProvider()
	suggestedCmd, err := ai.GenerateCommand(userPrompt)
	if err != nil {
		fmt.Printf("❌ Error while invoking Claude: %v\n", err)
		return 1
	}

	// --copy / -c: dump to clipboard and bail before the TUI.
	if copyToClipboard {
		fmt.Printf("\n👉 Suggested command:\n\033[1;32m%s\033[0m\n\n", suggestedCmd)
		if err := copyCommandToClipboard(suggestedCmd); err != nil {
			fmt.Printf("❌ Failed to copy to clipboard: %v\n", err)
			return 1
		}
		fmt.Println("📋 Command copied to clipboard.")
		recordHistory(userPrompt, suggestedCmd)
		return 0
	}

	// --yes / -y: run immediately, skip the TUI entirely.
	if skipPermission {
		fmt.Printf("\n👉 Suggested command:\n\033[1;32m%s\033[0m\n\n", suggestedCmd)
		executeCommand(suggestedCmd)
		recordHistory(userPrompt, suggestedCmd)
		return 0
	}

	// --no-tui: fall back to the plain y/N prompt for terminals without TUI support.
	if noTUI {
		fmt.Printf("\n👉 Suggested command:\n\033[1;32m%s\033[0m\n\n", suggestedCmd)
		fmt.Print("Do you want to run this command? [y/N]: ")
		input := readYesNo(stdin)
		if input == "y" || input == "yes" {
			executeCommand(suggestedCmd)
			recordHistory(userPrompt, suggestedCmd)
		} else {
			fmt.Println("Cancelled.")
		}
		return 0
	}

	// Default path: hand off to the Bubble Tea TUI for review / revision.
	finalCmd, shouldRun, err := runTUI(userPrompt, suggestedCmd, ai)
	if err != nil {
		fmt.Printf("❌ TUI error: %v\n", err)
		return 1
	}
	if !shouldRun {
		fmt.Println("Cancelled.")
		return 0
	}
	executeCommand(finalCmd)
	recordHistory(userPrompt, finalCmd)
	return 0
}

// recordHistory is a best-effort wrapper around saveHistory: a failure to
// persist a single entry shouldn't disrupt the foreground action the user
// just took, but we surface it on stderr so it isn't silently lost.
func recordHistory(prompt, command string) {
	if err := saveHistory(prompt, command); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  history not saved: %v\n", err)
	}
}

// readYesNo reads a single line from r and returns the lower-cased, trimmed
// answer. Pulled out of runRoot so the --no-tui branch is testable without
// rebinding os.Stdin globally.
func readYesNo(r io.Reader) string {
	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	return strings.ToLower(strings.TrimSpace(string(buf[:n])))
}

func executeCommand(shellCmd string) {
	fmt.Println("Running command...")
	if err := runShellCommand(shellCmd, os.Stdout, os.Stderr, os.Stdin); err != nil {
		fmt.Printf("\n❌ Command exited with error: %v\n", err)
	}
}

// runShellCommand pipes shellCmd through `bash -c` with the supplied IO
// streams. Split out from executeCommand so tests can capture stdout/stderr
// without redirecting the global os.Stdout.
func runShellCommand(shellCmd string, stdout, stderr io.Writer, stdin io.Reader) error {
	cmd := exec.Command("bash", "-c", shellCmd) // May need adaptation for Windows ("cmd /c" or "powershell -c").
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Stdin = stdin
	return cmd.Run()
}

// clipboardCommand picks the right OS-specific clipboard binary for goos.
// Separated from copyCommandToClipboard so tests can exercise every platform
// branch without juggling runtime.GOOS, which is fixed per build.
func clipboardCommand(goos string) (*exec.Cmd, error) {
	switch goos {
	case "darwin":
		return exec.Command("pbcopy"), nil
	case "linux":
		return exec.Command("xclip", "-selection", "clipboard"), nil
	case "windows":
		return exec.Command("clip"), nil
	default:
		return nil, fmt.Errorf("unsupported platform: %s", goos)
	}
}

func copyCommandToClipboard(text string) error {
	cmd, err := clipboardCommand(runtime.GOOS)
	if err != nil {
		return err
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
	// Setting Version triggers cobra's built-in --version / -v handling, which
	// short-circuits before Args validation, so `tai --version` works without a
	// positional prompt. We override the default template so commit/date show
	// alongside the version string.
	rootCmd.Version = version
	rootCmd.SetVersionTemplate(versionString())
	rootCmd.Flags().BoolP("version", "v", false, "Print version information and exit")

	rootCmd.Flags().BoolVarP(&skipPermission, "yes", "y", false, "Skip the confirmation prompt and run the command directly")
	rootCmd.Flags().BoolVarP(&copyToClipboard, "copy", "c", false, "Do not run the command, only copy it to the clipboard")
	rootCmd.Flags().BoolVar(&noTUI, "no-tui", false, "Use the plain y/N prompt instead of the Bubble Tea TUI (for terminals without TUI support)")
}
