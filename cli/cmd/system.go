package cmd

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show phone status (battery, wifi, storage)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		data, err := c.RunRaw(newCmd("status", nil, nil))
		if err != nil {
			return err
		}

		cyan.Println("── Device ──────────────────────────────────")
		if device, ok := data["device"].(map[string]interface{}); ok {
			fmt.Printf("  Model:    %v %v\n", device["manufacturer"], device["model"])
			fmt.Printf("  Android:  %v (SDK %v)\n", device["android"], device["sdk"])
		}

		cyan.Println("\n── Battery ─────────────────────────────────")
		if battery, ok := data["battery"].(map[string]interface{}); ok {
			pct := battery["percent"]
			status := battery["status"]
			temp := battery["temperature_c"]
			plugged := battery["plugged"]
			fmt.Printf("  Level:    %v%% (%v)\n", pct, status)
			fmt.Printf("  Plugged:  %v\n", plugged)
			fmt.Printf("  Temp:     %v°C\n", temp)
			fmt.Printf("  Health:   %v\n", battery["health"])
		}

		cyan.Println("\n── Storage ─────────────────────────────────")
		if storage, ok := data["storage"].(map[string]interface{}); ok {
			total := toGB(storage["total_bytes"])
			free := toGB(storage["free_bytes"])
			used := storage["used_percent"]
			fmt.Printf("  Total:    %.1f GB\n", total)
			fmt.Printf("  Free:     %.1f GB\n", free)
			fmt.Printf("  Used:     %v%%\n", used)
		}

		cyan.Println("\n── WiFi ─────────────────────────────────────")
		if wifi, ok := data["wifi"].(map[string]interface{}); ok {
			fmt.Printf("  SSID:     %v\n", wifi["ssid"])
			fmt.Printf("  IP:       %v\n", wifi["ip"])
			fmt.Printf("  Signal:   %v dBm\n", wifi["rssi"])
		}
		return nil
	},
}

var batteryCmd = &cobra.Command{
	Use:   "battery",
	Short: "Show detailed battery information",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		data, err := c.RunRaw(newCmd("battery", nil, nil))
		if err != nil {
			return err
		}
		fmt.Printf("Level:       %v%%\n", data["percent"])
		fmt.Printf("Status:      %v\n", data["status"])
		fmt.Printf("Plugged:     %v\n", data["plugged"])
		fmt.Printf("Health:      %v\n", data["health"])
		fmt.Printf("Temperature: %v°C\n", data["temperature_c"])
		fmt.Printf("Voltage:     %v mV\n", data["voltage_mv"])
		return nil
	},
}

var locationCmd = &cobra.Command{
	Use:   "location",
	Short: "Get current GPS coordinates",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		data, err := c.RunRaw(newCmd("location", nil, nil))
		if err != nil {
			return err
		}
		fmt.Printf("Latitude:   %v\n", data["latitude"])
		fmt.Printf("Longitude:  %v\n", data["longitude"])
		fmt.Printf("Accuracy:   %v m\n", data["accuracy"])
		fmt.Printf("Altitude:   %v m\n", data["altitude"])
		fmt.Printf("Provider:   %v\n", data["provider"])
		fmt.Printf("Maps:       %v\n", data["maps_url"])
		return nil
	},
}

var screenshotCmd = &cobra.Command{
	Use:   "screenshot [output-file]",
	Short: "Take a screenshot and download it",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		fmt.Println("Taking screenshot...")
		data, err := c.RunRaw(newCmd("screenshot", nil, nil))
		if err != nil {
			return err
		}

		content, ok := data["content"].(string)
		if !ok {
			return fmt.Errorf("unexpected response")
		}
		imgBytes, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return fmt.Errorf("decoding image: %w", err)
		}

		outFile := fmt.Sprintf("screenshot_%s.png",
			time.Now().Format("20060102_150405"))
		if len(args) == 1 {
			outFile = args[0]
		}

		if err := os.WriteFile(outFile, imgBytes, 0644); err != nil {
			return fmt.Errorf("writing file: %w", err)
		}
		green.Printf("Saved: %s (%d bytes)\n", outFile, len(imgBytes))
		return nil
	},
}

var volumeCmd = &cobra.Command{
	Use:   "volume [get | set <0-100>] [--stream music|ring|alarm]",
	Short: "Get or set phone volume",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		stream, _ := cmd.Flags().GetString("stream")
		flags := map[string]string{}
		if stream != "" {
			flags["stream"] = stream
		}

		subCmd := "get"
		if len(args) > 0 {
			subCmd = args[0]
		}
		cmdArgs := append([]string{subCmd}, args[1:]...)

		data, err := c.RunRaw(newCmd("volume", cmdArgs, flags))
		if err != nil {
			return err
		}

		if pct, ok := data["percent"]; ok {
			fmt.Printf("Volume: %v%% (%v / %v)\n", pct, data["current"], data["max"])
		} else {
			fmt.Printf("Volume set to %v / %v\n", data["set"], data["max"])
		}
		return nil
	},
}

var brightnessCmd = &cobra.Command{
	Use:   "brightness [get | set <0-100>]",
	Short: "Get or set screen brightness",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		subCmd := "get"
		if len(args) > 0 {
			subCmd = args[0]
		}

		data, err := c.RunRaw(newCmd("brightness", append([]string{subCmd}, args[1:]...), nil))
		if err != nil {
			return err
		}
		if pct, ok := data["percent"]; ok {
			fmt.Printf("Brightness: %v%%\n", pct)
		} else {
			fmt.Printf("Brightness set to %v%%\n", data["set"])
		}
		return nil
	},
}

var dndCmd = &cobra.Command{
	Use:   "dnd [on | off | priority | status]",
	Short: "Control Do Not Disturb mode",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		subCmd := "status"
		if len(args) > 0 {
			subCmd = args[0]
		}

		data, err := c.RunRaw(newCmd("dnd", []string{subCmd}, nil))
		if err != nil {
			return err
		}
		fmt.Printf("DND: %v\n", data["dnd"])
		return nil
	},
}

var wifiCmd = &cobra.Command{
	Use:   "wifi [status | list]",
	Short: "WiFi information",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		subCmd := "status"
		if len(args) > 0 {
			subCmd = args[0]
		}

		data, err := c.RunRaw(newCmd("wifi", []string{subCmd}, nil))
		if err != nil {
			return err
		}

		if subCmd == "status" {
			enabled := data["enabled"]
			ssid := data["ssid"]
			ip := data["ip"]
			rssi := data["rssi"]
			fmt.Printf("Enabled: %v\nSSID:    %v\nIP:      %v\nRSSI:    %v dBm\n", enabled, ssid, ip, rssi)
		} else if nets, ok := data["networks"].([]interface{}); ok {
			for _, n := range nets {
				if net, ok := n.(map[string]interface{}); ok {
					fmt.Printf("  %-30s  %v dBm\n", net["ssid"], net["level"])
				}
			}
		}
		return nil
	},
}

var clipboardCmd = &cobra.Command{
	Use:   "clipboard [get | set <text>]",
	Short: "Get or set clipboard contents",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		subCmd := "get"
		if len(args) > 0 {
			subCmd = args[0]
		}

		data, err := c.RunRaw(newCmd("clipboard", append([]string{subCmd}, args[1:]...), nil))
		if err != nil {
			return err
		}
		if subCmd == "get" {
			fmt.Printf("%v\n", data["text"])
		} else {
			green.Printf("Clipboard set to: %v\n", data["set"])
		}
		return nil
	},
}

func toGB(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n / 1e9
	case int64:
		return float64(n) / 1e9
	}
	return 0
}

func init() {
	volumeCmd.Flags().String("stream", "music", "audio stream: music, ring, alarm, call")
	_ = filepath.Join // keep import
}
