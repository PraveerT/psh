package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var notifsCmd = &cobra.Command{
	Use:   "notifs",
	Short: "List, watch, or clear notifications",
	Long: `Manage phone notifications.

Examples:
  psh notifs                    List recent notifications
  psh notifs --app slack        Filter by app name
  psh notifs --clear slack      Clear Slack notifications
  psh notifs --clear-all        Clear all notifications`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		app, _ := cmd.Flags().GetString("app")
		clear, _ := cmd.Flags().GetString("clear")
		clearAll, _ := cmd.Flags().GetBool("clear-all")
		limit, _ := cmd.Flags().GetInt("limit")

		flags := map[string]string{}
		if app != "" {
			flags["app"] = app
		}
		if clear != "" {
			flags["clear"] = clear
		}
		if clearAll {
			flags["clear-all"] = "true"
		}
		if limit != 50 {
			flags["limit"] = fmt.Sprintf("%d", limit)
		}

		data, err := c.RunRaw(newCmd("notifs", nil, flags))
		if err != nil {
			return err
		}

		if clear != "" || clearAll {
			green.Printf("Cleared %v notification(s)\n", data["cleared"])
			return nil
		}

		notifs, ok := data["notifications"].([]interface{})
		if !ok || len(notifs) == 0 {
			dim.Println("No notifications")
			return nil
		}

		fmt.Printf("%v notification(s):\n\n", data["count"])
		for _, n := range notifs {
			notif, ok := n.(map[string]interface{})
			if !ok {
				continue
			}
			pkg := notif["app"].(string)
			title := str(notif["title"])
			text := str(notif["text"])
			ts := int64(notif["time"].(float64))
			t := time.Unix(ts/1000, 0)

			cyan.Printf("  %s", pkg)
			dim.Printf("  %s\n", t.Format("15:04"))
			if title != "" {
				fmt.Printf("  %s\n", bold.Sprint(title))
			}
			if text != "" {
				// Truncate long notifications
				if len(text) > 120 {
					text = text[:117] + "..."
				}
				fmt.Printf("  %s\n", text)
			}
			fmt.Println()
		}
		return nil
	},
}

func init() {
	notifsCmd.Flags().String("app", "", "filter by app name/package")
	notifsCmd.Flags().String("clear", "", "clear notifications from this app")
	notifsCmd.Flags().Bool("clear-all", false, "clear all notifications")
	notifsCmd.Flags().Int("limit", 50, "max notifications to show")
}

func str(v interface{}) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}
