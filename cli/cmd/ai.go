package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/phonessh/psh/client"
	"github.com/spf13/cobra"
)

// ── System prompt ─────────────────────────────────────────────────────────────

const systemPrompt = `You are an AI assistant for PhoneSSH (psh) — a tool that lets users control their Android phone from the terminal.

IMPORTANT — AGENTIC MODE: You operate in rounds. For tasks that require seeing the screen:
• Round 1: Output ONLY "psh screenshot" to capture the current screen
• You will then be sent the actual screenshot image
• Round 2: Analyze the image carefully and output the precise commands

PREFERRED approach for clicking UI elements:
1. Run "psh ui dump" to get all labelled elements with their text
2. Use "psh click <text>" to click by element text — NO coordinates needed
3. Only use "psh tap <x> <y>" for elements with no text (images, unlabelled icons)

When you need to click something with visible text/label:
- ALWAYS prefer: psh ui dump → psh click "element text"
- Example: to click "Subscribe" button → psh click "Subscribe"

When you MUST take a screenshot (element has no text, purely visual):
- "tap the first image/video thumbnail" (no text label)
- Any task where the element has no visible text or description

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
- psh ui dump
- psh click <text-or-description>
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

User: "go back"
Response:
psh key back

Only output valid psh commands. If you cannot map the request to commands, output: # cannot map to psh commands: <reason>`

// ── Cobra command ─────────────────────────────────────────────────────────────

var aiNoContext bool
var aiClear bool

var aiCmd = &cobra.Command{
	Use:   "ai <natural-language-instruction>",
	Short: "Control your phone with natural language (powered by Claude)",
	Long: `Use natural language to control your phone via Claude AI.

Conversation context is saved between calls so Claude remembers what it
just did. Use --clear to start a fresh conversation.

For visual tasks Claude will take a screenshot, analyze it, then tap.

Requires the 'claude' CLI (claude.ai/code).

Examples:
  psh ai "open youtube and search for cats"
  psh ai "open the first video"
  psh ai "scroll down"
  psh ai --clear`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if aiClear {
			if err := clearContext(); err != nil {
				return err
			}
			green.Println("conversation context cleared")
			if len(args) == 0 {
				return nil
			}
		}
		if len(args) == 0 {
			return fmt.Errorf("provide a natural language instruction, or --clear to reset context")
		}
		query := strings.Join(args, " ")
		return runAI(query, aiNoContext)
	},
}

func init() {
	aiCmd.Flags().BoolVar(&aiClear, "clear", false, "clear conversation context and start fresh")
	aiCmd.Flags().BoolVar(&aiNoContext, "no-context", false, "ignore saved context for this call")
}

// ── Agentic runner ────────────────────────────────────────────────────────────

func runAI(query string, noContext bool) error {
	c, _, err := getClient()
	if err != nil {
		return err
	}
	defer c.Close()
	return runAgenticLoop(query, c, noContext)
}

func runAgenticLoop(query string, c *client.Client, noContext bool) error {
	var history []claudeCtxMsg
	if !noContext {
		history, _ = loadContext()
		if len(history) > 0 {
			dim.Printf("  [context: %d messages]\n", len(history))
		}
	}

	fmt.Println()

	// Round 1: get initial commands (may be "psh screenshot")
	commands, err := askClaude(history, query, "")
	if err != nil {
		return err
	}

	firstResponse := strings.Join(commands, "\n")
	screenshotB64, screenshotDims := executeCommands(c, commands, nil)

	finalResponse := firstResponse

	if screenshotB64 != "" {
		// Decode and save screenshot to a temp file
		imgData, err := base64.StdEncoding.DecodeString(screenshotB64)
		if err != nil {
			return fmt.Errorf("decoding screenshot: %w", err)
		}
		tmp, err := os.CreateTemp("", "psh-screenshot-*.png")
		if err != nil {
			return fmt.Errorf("creating temp file: %w", err)
		}
		tmp.Write(imgData)
		tmp.Close()
		defer os.Remove(tmp.Name())

		// Build history including the screenshot round so Claude has context
		roundHistory := append(append([]claudeCtxMsg{}, history...),
			claudeCtxMsg{Role: "user", Content: query},
			claudeCtxMsg{Role: "assistant", Content: firstResponse},
		)

		// Round 2: tell Claude the exact screen resolution so coordinates are correct
		dimsHint := ""
		if screenshotDims != "" {
			dimsHint = fmt.Sprintf("\nIMPORTANT: The phone screen is %s pixels. All tap/swipe coordinates must be in this pixel space.", screenshotDims)
		}
		round2 := fmt.Sprintf(
			"A screenshot of the phone screen has been saved to: %s%s\n\nUse the Read tool to view it, analyze what is on screen, then output the precise psh commands to complete the task.",
			tmp.Name(), dimsHint,
		)
		commands2, err := askClaudeWithRead(roundHistory, round2, filepath.Dir(tmp.Name()))
		if err != nil {
			return err
		}

		finalResponse = strings.Join(commands2, "\n")
		executeCommands(c, commands2, nil)
	}

	// Persist context: save user query + final commands (not screenshot internals)
	if !noContext && finalResponse != "" {
		updated := append(history,
			claudeCtxMsg{Role: "user", Content: query},
			claudeCtxMsg{Role: "assistant", Content: finalResponse},
		)
		// Keep last 40 messages (~20 exchanges)
		if len(updated) > 40 {
			updated = updated[len(updated)-40:]
		}
		if err := saveContext(updated); err != nil {
			dim.Printf("  (context save error: %v)\n", err)
		}
	}

	return nil
}

// ── Claude CLI ────────────────────────────────────────────────────────────────

// askClaude calls `claude -p` with the full conversation history in the prompt.
// If screenshotPath is set, passes it via --image for vision analysis.
func askClaude(history []claudeCtxMsg, query string, screenshotPath string) ([]string, error) {
	prompt := buildPrompt(history, query)

	args := []string{"-p", prompt}
	if screenshotPath != "" {
		args = append(args, "--image", screenshotPath)
	}

	out, err := exec.Command("claude", args...).Output()
	if err != nil {
		if _, which := exec.LookPath("claude"); which != nil {
			return nil, fmt.Errorf("'claude' CLI not found — install from claude.ai/code")
		}
		return nil, fmt.Errorf("running claude: %w", err)
	}

	return parseLines(strings.TrimSpace(string(out))), nil
}

// askClaudeWithRead calls claude -p with the Read tool allowed so Claude can
// open the screenshot image file and see it visually before responding.
func askClaudeWithRead(history []claudeCtxMsg, query string, allowDir string) ([]string, error) {
	prompt := buildPrompt(history, query)

	args := []string{
		"-p", prompt,
		"--allowedTools", "Read",
		"--add-dir", allowDir,
	}

	out, err := exec.Command("claude", args...).Output()
	if err != nil {
		if _, which := exec.LookPath("claude"); which != nil {
			return nil, fmt.Errorf("'claude' CLI not found — install from claude.ai/code")
		}
		return nil, fmt.Errorf("running claude (vision): %w", err)
	}

	return parseLines(strings.TrimSpace(string(out))), nil
}

// buildPrompt formats the system prompt + conversation history + new query.
func buildPrompt(history []claudeCtxMsg, query string) string {
	var sb strings.Builder
	sb.WriteString(systemPrompt)

	if len(history) > 0 {
		sb.WriteString("\n\n[Conversation history]\n")
		for _, m := range history {
			if m.Role == "user" {
				sb.WriteString("User: " + m.Content + "\n")
			} else {
				sb.WriteString("Assistant:\n" + m.Content + "\n")
			}
		}
		sb.WriteString("[End of conversation history]\n")
	}

	sb.WriteString("\nUser: " + query)
	return sb.String()
}

// ── Context persistence ───────────────────────────────────────────────────────

type claudeCtxMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func contextFilePath() (string, error) {
	dir, err := client.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ai_context.json"), nil
}

func loadContext() ([]claudeCtxMsg, error) {
	path, err := contextFilePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var msgs []claudeCtxMsg
	if err := json.Unmarshal(data, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

func saveContext(msgs []claudeCtxMsg) error {
	path, err := contextFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(msgs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func clearContext() error {
	path, err := contextFilePath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ── Command execution ─────────────────────────────────────────────────────────

// executeCommands runs parsed psh command strings against the phone.
// Returns base64 PNG and display dimensions ("WxH") if a screenshot was taken.
func executeCommands(c *client.Client, commands []string, _ interface{}) (screenshotB64, screenshotDims string) {

	for _, rawCmd := range commands {
		if strings.HasPrefix(rawCmd, "#") {
			dim.Printf("  %s\n", rawCmd)
			continue
		}

		parts := parseCommand(rawCmd)
		// Skip any line that isn't a psh command (e.g. Claude's explanatory text)
		if len(parts) < 2 || parts[0] != "psh" {
			if len(parts) > 0 {
				dim.Printf("  %s\n", rawCmd)
			}
			continue
		}
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

		if subCmd == "screenshot" {
			if b64, ok := result.Data["content"].(string); ok {
				screenshotB64 = b64
				w, _ := result.Data["display_width"].(float64)
				h, _ := result.Data["display_height"].(float64)
				if w > 0 && h > 0 {
					screenshotDims = fmt.Sprintf("%.0fx%.0f", w, h)
					dim.Printf("  screenshot captured (%s) — analyzing...\n", screenshotDims)
				} else {
					dim.Println("  screenshot captured — analyzing...")
				}
			}
			continue
		}

		printResultSummary(subCmd, result.Data)
	}

	return
}

// ── Helpers ───────────────────────────────────────────────────────────────────

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
	default:
		if errMsg, ok := data["error"]; ok {
			red.Printf("  %v\n", errMsg)
		} else {
			green.Println("  done")
		}
	}
}
