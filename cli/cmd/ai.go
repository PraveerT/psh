package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/phonessh/psh/client"
	"github.com/spf13/cobra"
)

// ── System prompt ─────────────────────────────────────────────────────────────

const systemPrompt = `You are an AI assistant for PhoneSSH (psh) — a tool that lets users control their Android phone from the terminal.

IMPORTANT — AGENTIC MODE: You operate in rounds. For tasks that require seeing the screen:
• Round 1: Output ONLY "psh screenshot" to capture the current screen
• The screenshot will be sent to you as an image
• Round 2: Analyze the image carefully and output the precise commands

When you MUST take a screenshot first (screen state unknown):
- "open the first/top/bottom [item]"
- "tap the [button/video/icon/link]"
- "click on [element]"
- Any task where coordinates depend on what's currently on screen

When you do NOT need a screenshot (these are deterministic):
- "search youtube for X"    → psh open "https://www.youtube.com/results?search_query=X"
- "launch spotify"          → psh apps launch spotify
- "go back"                 → psh key back
- "go home"                 → psh key home
- "send a text to X"        → psh sms send ...

When given a screenshot: look at the actual pixel positions of UI elements and output precise coordinates.
Typical screen widths: 1080 (most phones) or 1440 (high-res). Center x ≈ width/2.

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

User: "open the first video on the screen"
Response (round 1 — need visual context):
psh screenshot

[Screenshot provided]
Response (round 2 — tap based on what you see):
psh tap 540 420

User: "tap the search bar and type cats"
Response (round 1):
psh screenshot

[Screenshot provided]
Response (round 2):
psh tap 540 180
psh type "cats"

User: "search youtube for cats"
Response (no screenshot needed):
psh open "https://www.youtube.com/results?search_query=cats"

User: "clear slack notifications and turn on do not disturb"
Response:
psh notifs --clear slack
psh dnd on

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

// ── Cobra command ─────────────────────────────────────────────────────────────

var aiCmd = &cobra.Command{
	Use:   "ai <natural-language-instruction>",
	Short: "Control your phone with natural language (powered by Claude)",
	Long: `Use natural language to control your phone via Claude AI.

For visual tasks (e.g. "tap the first video"), Claude will automatically
take a screenshot, analyze it, and output precise tap coordinates.

Requires ANTHROPIC_API_KEY env var, OR the 'claude' CLI installed.

Examples:
  psh ai "open the first video on the screen"
  psh ai "clear my slack notifications"
  psh ai "search youtube for lofi music"
  psh ai "turn on do not disturb and set volume to 20"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		return runAI(query)
	},
}

// ── Agentic runner ────────────────────────────────────────────────────────────

func runAI(query string) error {
	apiKey := anthropicKey()

	// Connect to phone once, reuse for all rounds
	c, _, err := getClient()
	if err != nil {
		return err
	}
	defer c.Close()

	if apiKey != "" {
		return runAgenticLoop(query, apiKey, c)
	}

	// Fallback: single-pass via claude CLI (no vision)
	commands, err := askClaudeCLI(query)
	if err != nil {
		return fmt.Errorf("Claude error: %w", err)
	}
	fmt.Println()
	executeCommands(c, commands, nil)
	return nil
}

// runAgenticLoop runs Claude in a multi-turn loop with optional vision.
// Round 1: text query → commands (may include "psh screenshot")
// Round 2: screenshot image → follow-up commands
func runAgenticLoop(query, apiKey string, c *client.Client) error {
	messages := []claudeMsg{
		{Role: "user", Content: query},
	}

	fmt.Println()

	for round := 0; round < 3; round++ {
		resp, err := callClaude(apiKey, messages)
		if err != nil {
			return fmt.Errorf("Claude API: %w", err)
		}

		commands := parseLines(resp)

		// Add assistant turn to history
		messages = append(messages, claudeMsg{Role: "assistant", Content: resp})

		// Execute commands; capture screenshot data if taken
		screenshotB64 := executeCommands(c, commands, nil)

		if screenshotB64 == "" {
			// No screenshot taken — we're done
			break
		}

		// Pass the screenshot image to Claude for round 2
		messages = append(messages, claudeMsg{
			Role: "user",
			Content: []contentBlock{
				{
					Type: "image",
					Source: &imageSource{
						Type:      "base64",
						MediaType: "image/png",
						Data:      screenshotB64,
					},
				},
				{
					Type: "text",
					Text: "Here is the current screenshot of the phone screen. Analyze it and output the precise psh commands to complete the task.",
				},
			},
		})
	}

	return nil
}

// executeCommands runs a list of psh command strings. Returns the base64 PNG
// from a screenshot command if one was executed, otherwise "".
func executeCommands(c *client.Client, commands []string, _ interface{}) string {
	var screenshotB64 string

	for _, rawCmd := range commands {
		if strings.HasPrefix(rawCmd, "#") {
			dim.Printf("  %s\n", rawCmd)
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

		// Capture screenshot data for vision round
		if subCmd == "screenshot" {
			if b64, ok := result.Data["content"].(string); ok {
				screenshotB64 = b64
				dim.Println("  screenshot captured — analyzing...")
			}
			continue
		}

		printResultSummary(subCmd, result.Data)
	}

	return screenshotB64
}

// ── Anthropic API (HTTP) ──────────────────────────────────────────────────────

type claudeMsg struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or []contentBlock
}

type contentBlock struct {
	Type   string       `json:"type"`
	Text   string       `json:"text,omitempty"`
	Source *imageSource `json:"source,omitempty"`
}

type imageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type claudeRequest struct {
	Model     string      `json:"model"`
	MaxTokens int         `json:"max_tokens"`
	System    string      `json:"system"`
	Messages  []claudeMsg `json:"messages"`
}

type claudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func callClaude(apiKey string, messages []claudeMsg) (string, error) {
	reqBody := claudeRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
		System:    systemPrompt,
		Messages:  messages,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	httpClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var cr claudeResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}
	if cr.Error != nil {
		return "", fmt.Errorf("API error: %s", cr.Error.Message)
	}
	if len(cr.Content) == 0 {
		return "", fmt.Errorf("empty response from Claude")
	}
	return cr.Content[0].Text, nil
}

func anthropicKey() string {
	if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
		return k
	}
	cfg, err := client.LoadConfig()
	if err == nil && cfg.AnthropicKey != "" {
		return cfg.AnthropicKey
	}
	return ""
}

// ── Claude CLI fallback (no vision) ──────────────────────────────────────────

func askClaudeCLI(query string) ([]string, error) {
	prompt := systemPrompt + "\n\nUser: " + query

	out, err := exec.Command("claude", "-p", prompt).Output()
	if err != nil {
		if _, which := exec.LookPath("claude"); which != nil {
			return nil, fmt.Errorf("no ANTHROPIC_API_KEY set and 'claude' CLI not found\n\nSet your API key:  $env:ANTHROPIC_API_KEY=\"sk-ant-...\"")
		}
		return nil, fmt.Errorf("running claude: %w", err)
	}

	return parseLines(strings.TrimSpace(string(out))), nil
}

func parseLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// ── Helpers (unchanged) ───────────────────────────────────────────────────────

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
