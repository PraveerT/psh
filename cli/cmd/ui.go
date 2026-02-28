package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var openCmd = &cobra.Command{
	Use:   "open <url-or-deep-link>",
	Short: "Open a URL or deep link on the phone",
	Long: `Open any URL or Android deep link on the phone.

Examples:
  psh open "https://www.youtube.com/results?search_query=cats"
  psh open "https://maps.google.com/?q=coffee+near+me"
  psh open "vnd.youtube://results?search_query=lofi+music"
  psh open "spotify://search/chill"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _, err := getClient()
		if err != nil {
			return err
		}
		defer c.Close()

		result, err := c.Run(newCmd("open", args, nil))
		if err != nil {
			return err
		}
		if !result.Ok {
			return fmt.Errorf("%s", result.Error)
		}
		green.Printf("opened: %s\n", args[0])
		return nil
	},
}

var tapCmd = &cobra.Command{
	Use:   "tap <x> <y>",
	Short: "Tap the screen at coordinates",
	Long: `Tap the phone screen at the given pixel coordinates.

Use 'psh screenshot' first to identify coordinates.

Examples:
  psh tap 540 1200
  psh tap 100 500`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := strconv.ParseFloat(args[0], 32); err != nil {
			return fmt.Errorf("invalid x coordinate: %s", args[0])
		}
		if _, err := strconv.ParseFloat(args[1], 32); err != nil {
			return fmt.Errorf("invalid y coordinate: %s", args[1])
		}

		c, _, err := getClient()
		if err != nil {
			return err
		}
		defer c.Close()

		result, err := c.Run(newCmd("tap", args, nil))
		if err != nil {
			return err
		}
		if !result.Ok {
			return fmt.Errorf("%s", result.Error)
		}
		green.Printf("tapped (%s, %s)\n", args[0], args[1])
		return nil
	},
}

var swipeDuration int

var swipeCmd = &cobra.Command{
	Use:   "swipe <x1> <y1> <x2> <y2>",
	Short: "Swipe the screen from one point to another",
	Long: `Perform a swipe gesture on the phone screen.

Examples:
  psh swipe 540 1500 540 500          # scroll up
  psh swipe 540 500 540 1500          # scroll down
  psh swipe 100 960 900 960           # swipe right
  psh swipe 540 1200 540 400 --duration 800`,
	Args: cobra.ExactArgs(4),
	RunE: func(cmd *cobra.Command, args []string) error {
		for i, a := range args {
			if _, err := strconv.ParseFloat(a, 32); err != nil {
				return fmt.Errorf("invalid coordinate at position %d: %s", i+1, a)
			}
		}

		flags := map[string]string{}
		if swipeDuration != 300 {
			flags["duration"] = strconv.Itoa(swipeDuration)
		}

		c, _, err := getClient()
		if err != nil {
			return err
		}
		defer c.Close()

		result, err := c.Run(newCmd("swipe", args, flags))
		if err != nil {
			return err
		}
		if !result.Ok {
			return fmt.Errorf("%s", result.Error)
		}
		green.Printf("swiped (%s,%s) → (%s,%s)\n", args[0], args[1], args[2], args[3])
		return nil
	},
}

var typeCmd = &cobra.Command{
	Use:   "type <text>",
	Short: "Type text into the focused input field",
	Long: `Set text in the currently focused input field on the phone.

Note: Tap into a text field first (psh tap <x> <y>) before typing.
This replaces the entire field content.

Examples:
  psh type "cats"
  psh type "hello world"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		text := strings.Join(args, " ")

		c, _, err := getClient()
		if err != nil {
			return err
		}
		defer c.Close()

		result, err := c.Run(newCmd("type", []string{text}, nil))
		if err != nil {
			return err
		}
		if !result.Ok {
			return fmt.Errorf("%s", result.Error)
		}
		green.Printf("typed: %s\n", text)
		return nil
	},
}

var keyCmd = &cobra.Command{
	Use:   "key <back|home|recents|notifications>",
	Short: "Press a navigation or hardware key",
	Long: `Press a navigation or hardware key on the phone.

Available keys:
  back          — go back
  home          — go to home screen
  recents       — open recent apps
  notifications — open notification shade

Examples:
  psh key back
  psh key home
  psh key recents
  psh key notifications`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"back", "home", "recents", "notifications"},
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		switch key {
		case "back", "home", "recents", "notifications":
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown key %q — valid: back, home, recents, notifications\n", key)
			os.Exit(1)
		}

		c, _, err := getClient()
		if err != nil {
			return err
		}
		defer c.Close()

		result, err := c.Run(newCmd("key", []string{key}, nil))
		if err != nil {
			return err
		}
		if !result.Ok {
			return fmt.Errorf("%s", result.Error)
		}
		green.Printf("key: %s\n", key)
		return nil
	},
}

func init() {
	swipeCmd.Flags().IntVar(&swipeDuration, "duration", 300, "swipe duration in milliseconds")
}
