package cmd

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/phonessh/psh/client"
	"github.com/spf13/cobra"
)

var pairCmd = &cobra.Command{
	Use:   "pair [psh://pair?... URL]",
	Short: "Pair with a phone by scanning or pasting the QR payload",
	Long: `Pair psh with your phone.

Either:
  1. Run 'psh pair' and paste the URL shown on your phone's screen
  2. Run 'psh pair psh://pair?host=...&port=...&token=...&name=...'`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var pairURL string

		if len(args) == 1 {
			pairURL = args[0]
		} else {
			fmt.Println("On your phone: open PhoneSSH and copy the pairing URL shown below the QR code.")
			fmt.Print("Paste the pairing URL here: ")
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				pairURL = strings.TrimSpace(scanner.Text())
			}
		}

		if pairURL == "" {
			return fmt.Errorf("no pairing URL provided")
		}

		dev, err := parsePairURL(pairURL)
		if err != nil {
			return fmt.Errorf("invalid pairing URL: %w", err)
		}

		fmt.Printf("Connecting to %s (%s:%d)...\n", dev.Name, dev.Host, dev.Port)
		c, err := client.Connect(dev)
		if err != nil {
			return err
		}
		defer c.Close()

		// Test with a status command
		result, err := c.RunRaw(newCmd("status", nil, nil))
		if err != nil {
			return fmt.Errorf("test command failed: %w", err)
		}

		cfg, err := client.LoadConfig()
		if err != nil {
			cfg = &client.Config{}
		}
		cfg.AddOrUpdateDevice(*dev)
		if err := client.SaveConfig(cfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		green.Printf("\nPaired successfully with %s!\n", dev.Name)

		if deviceInfo, ok := result["device"].(map[string]interface{}); ok {
			fmt.Printf("  Model:   %v\n", deviceInfo["model"])
			fmt.Printf("  Android: %v\n", deviceInfo["android"])
		}

		fmt.Printf("\nRun 'psh status' to get started.\n")
		return nil
	},
}

var devicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List paired devices",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := client.LoadConfig()
		if err != nil {
			return err
		}
		if len(cfg.Devices) == 0 {
			fmt.Println("No devices paired. Run: psh pair")
			return nil
		}
		for _, d := range cfg.Devices {
			marker := "  "
			if d.Name == cfg.DefaultDevice {
				marker = "* "
			}
			fmt.Printf("%s%s  %s:%d\n", marker, d.Name, d.Host, d.Port)
		}
		return nil
	},
}

func parsePairURL(raw string) (*client.Device, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "psh" || u.Host != "pair" {
		return nil, fmt.Errorf("expected psh://pair?... URL, got: %s", raw)
	}

	q := u.Query()
	host := q.Get("host")
	portStr := q.Get("port")
	token := q.Get("token")
	name := q.Get("name")

	if host == "" || token == "" {
		return nil, fmt.Errorf("missing host or token in URL")
	}
	port := 8765
	if portStr != "" {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port: %s", portStr)
		}
	}
	if name == "" {
		name = host
	}

	return &client.Device{
		Name:  name,
		Host:  host,
		Port:  port,
		Token: token,
	}, nil
}
