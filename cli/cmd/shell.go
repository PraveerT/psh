package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/chzyer/readline"
	"github.com/phonessh/psh/client"
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Interactive psh shell — no 'psh' prefix needed",
	Long: `Start an interactive shell connected to your phone.

Type commands directly without the 'psh' prefix:

  pixel> status
  pixel> apps launch youtube
  pixel> open "https://www.youtube.com/results?search_query=cats"
  pixel> tap 540 1200
  pixel> key back
  pixel> exit

Use ↑/↓ arrow keys for command history.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, dev, err := getClient()
		if err != nil {
			return err
		}
		defer c.Close()

		rl, err := readline.NewEx(&readline.Config{
			Prompt:            dev.Name + "> ",
			HistoryLimit:      500,
			InterruptPrompt:   "^C",
			EOFPrompt:         "exit",
		})
		if err != nil {
			return fmt.Errorf("shell init: %w", err)
		}
		defer rl.Close()

		bold.Printf("\n  PhoneSSH shell — %s\n", dev.Name)
		dim.Printf("  'help' for commands · 'exit' to quit · ↑↓ for history\n\n")

		for {
			line, err := rl.Readline()
			if err == readline.ErrInterrupt {
				// Ctrl+C clears the line — don't exit
				continue
			}
			if err == io.EOF {
				// Ctrl+D
				fmt.Println()
				break
			}
			if err != nil {
				return err
			}

			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			switch line {
			case "exit", "quit":
				dim.Println("bye")
				return nil
			case "help":
				shellHelp()
				continue
			case "clear", "cls":
				fmt.Print("\033[H\033[2J")
				continue
			}

			parts := parseCommand(line)
			if len(parts) == 0 {
				continue
			}
			// Let user accidentally type "psh status" — strip the prefix
			if parts[0] == "psh" {
				parts = parts[1:]
			}
			if len(parts) == 0 {
				continue
			}

			subCmd := parts[0]
			cmdArgs := parts[1:]

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

			msg := client.CmdMsg{
				Type:  "cmd",
				ID:    fmt.Sprintf("%d", time.Now().UnixNano()),
				Cmd:   subCmd,
				Args:  pureArgs,
				Flags: flags,
			}

			result, err := c.Run(msg)
			if err != nil {
				red.Printf("  error: %v\n\n", err)
				continue
			}
			if !result.Ok {
				red.Printf("  error: %s\n\n", result.Error)
				continue
			}

			shellPrint(subCmd, pureArgs, result.Data)
		}

		return nil
	},
}

func shellPrint(cmd string, args []string, data map[string]interface{}) {
	switch cmd {
	case "status":
		if b, ok := data["battery"].(map[string]interface{}); ok {
			fmt.Printf("  battery   %v%% (%v)\n", b["percent"], b["status"])
		}
		if s, ok := data["storage"].(map[string]interface{}); ok {
			fmt.Printf("  storage   %v free of %v\n", s["free_human"], s["total_human"])
		}
		if w, ok := data["wifi"].(map[string]interface{}); ok {
			fmt.Printf("  wifi      %v\n", w["ssid"])
		}
		if r, ok := data["ringer"].(string); ok {
			fmt.Printf("  ringer    %v\n", r)
		}

	case "battery":
		fmt.Printf("  %v%% — %v\n", data["percent"], data["status"])
		if temp, ok := data["temperature"]; ok {
			fmt.Printf("  %v°C\n", temp)
		}

	case "location":
		fmt.Printf("  %.6f, %.6f\n", data["latitude"], data["longitude"])
		if url, ok := data["maps_url"].(string); ok {
			dim.Printf("  %s\n", url)
		}

	case "screenshot":
		if path, ok := data["path"].(string); ok {
			green.Printf("  saved: %s\n", path)
		} else {
			green.Println("  screenshot saved")
		}

	case "notifs":
		count, _ := data["count"].(float64)
		fmt.Printf("  %d notification(s)\n", int(count))
		if items, ok := data["notifications"].([]interface{}); ok {
			for _, n := range items {
				if m, ok := n.(map[string]interface{}); ok {
					fmt.Printf("  [%v] %v: %v\n", m["app"], m["title"], m["text"])
				}
			}
		}

	case "sms":
		if msgs, ok := data["messages"].([]interface{}); ok {
			for _, m := range msgs {
				if msg, ok := m.(map[string]interface{}); ok {
					fmt.Printf("  %v  %v\n", msg["address"], msg["body"])
				}
			}
		} else if sent, ok := data["sent"].(bool); ok && sent {
			green.Println("  message sent")
		}

	case "apps":
		sub := args[0:]
		if len(sub) > 0 && sub[0] == "list" {
			count, _ := data["count"].(float64)
			fmt.Printf("  %d apps\n", int(count))
			if apps, ok := data["apps"].([]interface{}); ok {
				for _, a := range apps {
					if app, ok := a.(map[string]interface{}); ok {
						dim.Printf("  %-30s %v\n", app["name"], app["package"])
					}
				}
			}
		} else {
			shellPrintGeneric(data)
		}

	case "volume":
		if pct, ok := data["percent"]; ok {
			fmt.Printf("  volume: %v%%\n", pct)
		} else {
			green.Println("  volume set")
		}

	case "brightness":
		if pct, ok := data["percent"]; ok {
			fmt.Printf("  brightness: %v%%\n", pct)
		} else {
			green.Println("  brightness set")
		}

	case "dnd":
		fmt.Printf("  DND: %v\n", data["dnd"])

	case "wifi":
		if ssid, ok := data["ssid"].(string); ok {
			fmt.Printf("  connected: %s\n", ssid)
		} else {
			shellPrintGeneric(data)
		}

	case "clipboard":
		if text, ok := data["text"].(string); ok {
			fmt.Printf("  %s\n", text)
		} else {
			green.Println("  clipboard set")
		}

	case "open", "tap", "swipe", "type", "key", "lock", "mkdir", "rm":
		green.Println("  done")

	case "ls":
		if entries, ok := data["entries"].([]interface{}); ok {
			for _, e := range entries {
				if entry, ok := e.(map[string]interface{}); ok {
					isDir, _ := entry["is_dir"].(bool)
					name, _ := entry["name"].(string)
					size, _ := entry["size"]
					if isDir {
						cyan.Printf("  %s/\n", name)
					} else {
						fmt.Printf("  %-40s %v\n", name, size)
					}
				}
			}
		}

	case "stat":
		for k, v := range data {
			fmt.Printf("  %-16s %v\n", k, v)
		}

	case "pull":
		if dest, ok := data["dest"].(string); ok {
			green.Printf("  saved: %s\n", dest)
		} else {
			green.Println("  done")
		}

	case "push":
		green.Println("  uploaded")

	default:
		shellPrintGeneric(data)
	}

	fmt.Println()
}

func shellPrintGeneric(data map[string]interface{}) {
	if errMsg, ok := data["error"]; ok {
		red.Printf("  %v\n", errMsg)
		return
	}
	// Pretty-print as indented JSON
	b, _ := json.MarshalIndent(data, "  ", "  ")
	fmt.Printf("%s\n", b)
}

func shellHelp() {
	fmt.Println()
	bold.Println("  Available commands:")
	fmt.Println()
	entries := [][]string{
		{"status", "device overview"},
		{"battery", "battery level"},
		{"location", "GPS location"},
		{"screenshot", "take screenshot"},
		{"ls <path>", "list files"},
		{"find <pattern> [path]", "search files"},
		{"pull <path> [dest]", "download file"},
		{"push <file> <path>", "upload file"},
		{"rm <path>", "delete file"},
		{"mkdir <path>", "create directory"},
		{"notifs [--app <name>]", "list notifications"},
		{"sms list [--unread]", "list SMS"},
		{"sms send <num> <msg>", "send SMS"},
		{"apps list [--filter x]", "list apps"},
		{"apps launch <name>", "launch app"},
		{"apps kill <name>", "kill app"},
		{"volume [get|set <0-100>]", "speaker volume"},
		{"brightness set <0-100>", "screen brightness"},
		{"dnd [on|off|status]", "do not disturb"},
		{"wifi status", "WiFi info"},
		{"clipboard [get|set]", "clipboard"},
		{"lock", "lock screen"},
		{"open <url>", "open URL / deep link"},
		{"tap <x> <y>", "tap screen"},
		{"swipe <x1> <y1> <x2> <y2>", "swipe gesture"},
		{"type <text>", "type into focused field"},
		{"key <back|home|recents|notifs>", "press nav key"},
		{"help", "show this help"},
		{"exit", "quit shell"},
	}
	for _, e := range entries {
		cyan.Printf("  %-36s", e[0])
		dim.Printf("%s\n", e[1])
	}
	fmt.Println()
}

