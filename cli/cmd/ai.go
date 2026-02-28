package cmd

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/phonessh/psh/client"
	"github.com/spf13/cobra"
)

const systemPrompt = `You are an AI assistant for PhoneSSH (psh) — a tool that lets users control their Android phone from the terminal.

When the user gives you a natural language instruction, respond with ONLY the psh commands to run, one per line. No explanation, no markdown, no code blocks — just the raw commands.

Available commands:
- psh status
- psh battery
- psh location
- psh screenshot
- psh ls <path>
- psh find <pattern> [path]
- psh pull <remote-path> [local-dir]
- psh push <local-file> <remote-path>
- psh notifs
- psh notifs --app <name>
- psh notifs --clear <app>
- psh notifs --clear-all
- psh sms list [--unread]
- psh sms send <number> "<message>"
- psh apps list [--filter <name>]
- psh apps launch <name>
- psh apps kill <name>
- psh apps info <name>
- psh volume get
- psh volume set <0-100>
- psh brightness set <0-100>
- psh dnd on
- psh dnd off
- psh dnd status
- psh wifi status
- psh clipboard get
- psh clipboard set "<text>"
- psh open <url-or-deep-link>
- psh tap <x> <y>
- psh swipe <x1> <y1> <x2> <y2> [--duration <ms>]
- psh type "<text>"
- psh key <back|home|recents|notifications>

Examples:
User: "clear slack notifications and turn on do not disturb"
Response:
psh notifs --clear slack
psh dnd on

User: "what's my battery and storage"
Response:
psh status

User: "take a screenshot"
Response:
psh screenshot

User: "send a text to +1234567890 saying I'm on my way"
Response:
psh sms send +1234567890 "I'm on my way"

User: "search youtube for cats"
Response:
psh open "https://www.youtube.com/results?search_query=cats"

User: "open spotify and search for lofi music"
Response:
psh open "spotify://search/lofi music"

User: "go back"
Response:
psh key back

User: "go to home screen"
Response:
psh key home

Only output valid psh commands. If you cannot map the request to commands, output: # cannot map to psh commands: <reason>`

var aiCmd = &cobra.Command{
	Use:   "ai <natural-language-instruction>",
	Short: "Control your phone with natural language (powered by Claude)",
	Long: `Use natural language to control your phone via Claude AI.

Requires the 'claude' CLI to be installed and authenticated (claude.ai/code).

Examples:
  psh ai "clear my slack notifications"
  psh ai "what's my battery level and location"
  psh ai "turn on do not disturb and set volume to 20"
  psh ai "take a screenshot"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")

		commands, err := askClaude(query)
		if err != nil {
			return fmt.Errorf("Claude error: %w", err)
		}

		if len(commands) == 0 {
			return fmt.Errorf("Claude returned no commands")
		}

		fmt.Println()

		// Connect once and execute all commands
		c, _, err := getClient()
		if err != nil {
			return err
		}
		defer c.Close()

		for _, rawCmd := range commands {
			if strings.HasPrefix(rawCmd, "#") {
				dim.Printf("skip: %s\n", rawCmd)
				continue
			}

			parts := parseCommand(rawCmd)
			if len(parts) < 2 {
				continue
			}
			// parts[0] is "psh", parts[1] is the subcommand
			subCmd := parts[1]
			cmdArgs := parts[2:]

			flags := map[string]string{}
			pureArgs := []string{}
			i := 0
			for i < len(cmdArgs) {
				if strings.HasPrefix(cmdArgs[i], "--") {
					key := strings.TrimPrefix(cmdArgs[i], "--")
					if i+1 < len(cmdArgs) && !strings.HasPrefix(cmdArgs[i+1], "--") {
						flags[key] = cmdArgs[i+1]
						i += 2
					} else {
						flags[key] = "true"
						i++
					}
				} else {
					pureArgs = append(pureArgs, cmdArgs[i])
					i++
				}
			}

			cyan.Printf("→ %s\n", rawCmd)
			msg := client.CmdMsg{
				Type:  "cmd",
				ID:    fmt.Sprintf("%d", time.Now().UnixNano()),
				Cmd:   subCmd,
				Args:  pureArgs,
				Flags: flags,
			}

			result, err := c.Run(msg)
			if err != nil {
				red.Printf("  error: %v\n", err)
				continue
			}
			if !result.Ok {
				red.Printf("  error: %s\n", result.Error)
				continue
			}

			printResultSummary(subCmd, result.Data)
		}

		return nil
	},
}

// askClaude runs the claude CLI in print mode and returns a list of psh commands.
func askClaude(query string) ([]string, error) {
	prompt := systemPrompt + "\n\nUser: " + query

	out, err := exec.Command("claude", "-p", prompt).Output()
	if err != nil {
		// Try to give a helpful error if claude isn't installed
		if _, which := exec.LookPath("claude"); which != nil {
			return nil, fmt.Errorf("'claude' CLI not found — install it from claude.ai/code")
		}
		return nil, fmt.Errorf("running claude: %w", err)
	}

	text := strings.TrimSpace(string(out))
	var commands []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		commands = append(commands, line)
	}
	return commands, nil
}

func parseCommand(raw string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range raw {
		switch {
		case !inQuote && (r == '"' || r == '\''):
			inQuote = true
			quoteChar = r
		case inQuote && r == quoteChar:
			inQuote = false
		case !inQuote && r == ' ':
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func printResultSummary(cmd string, data map[string]interface{}) {
	switch cmd {
	case "status":
		if b, ok := data["battery"].(map[string]interface{}); ok {
			fmt.Printf("  Battery: %v%% (%v)\n", b["percent"], b["status"])
		}
		if w, ok := data["wifi"].(map[string]interface{}); ok {
			fmt.Printf("  WiFi: %v\n", w["ssid"])
		}
	case "battery":
		fmt.Printf("  %v%% — %v\n", data["percent"], data["status"])
	case "location":
		fmt.Printf("  %v, %v\n", data["latitude"], data["longitude"])
		fmt.Printf("  %v\n", data["maps_url"])
	case "notifs":
		fmt.Printf("  %v notification(s)\n", data["count"])
	case "dnd":
		fmt.Printf("  DND: %v\n", data["dnd"])
	case "volume":
		if pct, ok := data["percent"]; ok {
			fmt.Printf("  Volume: %v%%\n", pct)
		} else {
			fmt.Printf("  Volume set\n")
		}
	case "screenshot":
		green.Println("  Screenshot saved")
	default:
		if errMsg, ok := data["error"]; ok {
			red.Printf("  %v\n", errMsg)
		} else {
			green.Println("  done")
		}
	}
}

