// Package main is the gemini-cli entrypoint.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/puremachinery/gemini-cli-go/internal/version"
)

func main() {
	var (
		showVersion  bool
		showHelp     bool
		promptText   string
		approvalMode string
		yoloMode     bool
	)

	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&showVersion, "v", false, "print version and exit (shorthand)")
	flag.BoolVar(&showHelp, "help", false, "show help and exit")
	flag.BoolVar(&showHelp, "h", false, "show help and exit (shorthand)")
	flag.StringVar(&promptText, "prompt", "", "run in non-interactive (headless) mode with the given prompt")
	flag.StringVar(&promptText, "p", "", "run in non-interactive (headless) mode with the given prompt (shorthand)")
	flag.StringVar(&approvalMode, "approval-mode", "", "set tool approval mode: default, auto_edit, yolo, plan")
	flag.BoolVar(&yoloMode, "yolo", false, "auto-approve all tool calls (equivalent to --approval-mode=yolo)")
	flag.BoolVar(&yoloMode, "y", false, "auto-approve all tool calls (shorthand)")
	flag.Usage = usage
	flag.Parse()

	if showHelp {
		usage()
		return
	}

	if showVersion {
		fmt.Println(version.String())
		return
	}
	if yoloMode && strings.TrimSpace(approvalMode) != "" {
		fmt.Fprintln(os.Stderr, "Cannot use both --yolo (-y) and --approval-mode together. Use --approval-mode=yolo instead.")
		os.Exit(2)
	}

	args := flag.Args()
	isPiped := stdinIsPiped()
	isTTY := stdinIsTTY()
	promptTrimmed := strings.TrimSpace(promptText)
	promptValue := promptText
	if promptTrimmed != "" && len(args) > 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments with --prompt")
		os.Exit(2)
	}
	positionalPrompt := strings.Join(args, " ")
	if promptTrimmed != "" || isPiped || (!isTTY && positionalPrompt != "") {
		if promptTrimmed == "" {
			promptValue = positionalPrompt
		}
		stdinInput := ""
		if isPiped {
			var err error
			stdinInput, err = readStdinWithTimeout(stdinReadTimeout, maxStdinBytes)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
		combined := combinePrompt(stdinInput, promptValue)
		if strings.TrimSpace(combined) == "" {
			fmt.Fprintln(os.Stderr, "No input provided via stdin.")
			os.Exit(2)
		}
		if err := runHeadless(combined, approvalMode, yoloMode); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if positionalPrompt != "" && isTTY {
		input := io.MultiReader(strings.NewReader(positionalPrompt+"\n"), os.Stdin)
		if err := runInteractiveWithInput(input, approvalMode, yoloMode); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	if err := runInteractive(approvalMode, yoloMode); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func usage() {
	out := flag.CommandLine.Output()
	if _, err := fmt.Fprintln(out, "gemini-cli-go"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(out, ""); err != nil {
		return
	}
	if _, err := fmt.Fprintln(out, "Usage:"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(out, "  gemini-cli [--version] [--help] [query...]"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(out, "  gemini-cli"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(out, "  gemini-cli --prompt \"your prompt\""); err != nil {
		return
	}
	if _, err := fmt.Fprintln(out, "  echo \"input\" | gemini-cli"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(out, ""); err != nil {
		return
	}
	if _, err := fmt.Fprintln(out, "Flags:"); err != nil {
		return
	}
	flag.PrintDefaults()
	if _, err := fmt.Fprintln(out, ""); err != nil {
		return
	}
	printEnvironmentVariables(out)
}

func printEnvironmentVariables(w io.Writer) {
	if _, err := fmt.Fprintln(w, "Environment variables:"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  GEMINI_API_KEY                 API key for Gemini API auth"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  GEMINI_API_KEY_AUTH_MECHANISM  x-goog-api-key (default) or bearer"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  GEMINI_OAUTH_CLIENT_ID         Override OAuth client ID (advanced)"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  GEMINI_OAUTH_CLIENT_SECRET     Override OAuth client secret (advanced)"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  GOOGLE_CLOUD_PROJECT           Project ID for Code Assist/Vertex API calls"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  GOOGLE_APPLICATION_CREDENTIALS Path to OAuth credentials JSON"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  NO_BROWSER                     Set to true to use manual device code auth"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  OAUTH_CALLBACK_HOST            Host to bind for OAuth callback (default 127.0.0.1)"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  OAUTH_CALLBACK_PORT            Port to bind for OAuth callback (default random)"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  CODE_ASSIST_ENDPOINT           Override Code Assist API endpoint"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  CODE_ASSIST_API_VERSION        Override Code Assist API version"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  GEMINI_CLI_LOG_LEVEL           Enable structured logs (debug|info|warn|error)"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  GEMINI_CLI_LOG_FORMAT          Log format: text (default) or json"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  GEMINI_CLI_SYSTEM_SETTINGS_PATH Override system settings.json path"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "  GEMINI_CLI_SYSTEM_DEFAULTS_PATH Override system defaults path"); err != nil {
		return
	}
}
