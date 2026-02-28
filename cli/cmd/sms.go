package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

var smsCmd = &cobra.Command{
	Use:   "sms",
	Short: "Read or send SMS messages",
	Long: `Interact with SMS messages.

Examples:
  psh sms list               List recent messages
  psh sms list --unread      Show only unread messages
  psh sms list --from +1234  Filter by sender
  psh sms send +1234567890 "Running late"
  psh sms conversations      Show conversation threads`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Default: list
		return smsListRun(cmd, []string{"list"})
	},
}

var smsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List SMS messages",
	RunE:  smsListRun,
}

var smsSendCmd = &cobra.Command{
	Use:   "send <number> <message>",
	Short: "Send an SMS message",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		number := args[0]
		message := joinArgs(args[1:])

		fmt.Printf("Sending to %s: %q\n", number, message)
		data, err := c.RunRaw(newCmd("sms", append([]string{"send"}, args...), nil))
		if err != nil {
			return err
		}
		green.Printf("Sent (%v part(s))\n", data["parts"])
		return nil
	},
}

var smsConversationsCmd = &cobra.Command{
	Use:   "conversations",
	Short: "List SMS conversation threads",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		data, err := c.RunRaw(newCmd("sms", []string{"conversations"}, nil))
		if err != nil {
			return err
		}

		convos, ok := data["conversations"].([]interface{})
		if !ok || len(convos) == 0 {
			dim.Println("No conversations")
			return nil
		}

		for _, cv := range convos {
			convo, ok := cv.(map[string]interface{})
			if !ok {
				continue
			}
			ts := int64(convo["date"].(float64))
			t := time.Unix(ts/1000, 0)
			fmt.Printf("Thread %-6v  %s  %s\n",
				convo["thread_id"],
				dim.Sprint(t.Format("Jan 02")),
				str(convo["snippet"]))
		}
		return nil
	},
}

func init() {
	smsListCmd.Flags().Bool("unread", false, "show only unread messages")
	smsListCmd.Flags().String("from", "", "filter by sender number")
	smsListCmd.Flags().Int("limit", 30, "max messages to show")

	smsCmd.AddCommand(smsListCmd)
	smsCmd.AddCommand(smsSendCmd)
	smsCmd.AddCommand(smsConversationsCmd)
}

func smsListRun(cmd *cobra.Command, args []string) error {
	c, _ := mustConnect()
	defer c.Close()

	flags := map[string]string{}
	if unread, _ := cmd.Flags().GetBool("unread"); unread {
		flags["unread"] = "true"
	}
	if from, _ := cmd.Flags().GetString("from"); from != "" {
		flags["from"] = from
	}

	data, err := c.RunRaw(newCmd("sms", []string{"list"}, flags))
	if err != nil {
		return err
	}

	msgs, ok := data["messages"].([]interface{})
	if !ok || len(msgs) == 0 {
		dim.Println("No messages")
		return nil
	}

	fmt.Printf("%v message(s):\n\n", data["count"])
	for _, m := range msgs {
		msg, ok := m.(map[string]interface{})
		if !ok {
			continue
		}
		from := str(msg["from"])
		body := str(msg["body"])
		ts := int64(msg["time"].(float64))
		t := time.Unix(ts/1000, 0)
		msgType := str(msg["type"])
		read := msg["read"].(bool)

		prefix := "  "
		if !read {
			prefix = "● "
		}
		if msgType == "sent" {
			prefix = "→ "
		}

		if len(body) > 100 {
			body = body[:97] + "..."
		}

		fmt.Printf("%s%-16s  %s  %s\n",
			prefix,
			cyan.Sprint(from),
			dim.Sprint(t.Format("Jan 02 15:04")),
			body,
		)
	}
	return nil
}

func joinArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}
