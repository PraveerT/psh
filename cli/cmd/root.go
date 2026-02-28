package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"
	"github.com/phonessh/psh/client"
	"github.com/spf13/cobra"
)

var (
	flagDevice string
	flagHost   string
	flagPort   int
	flagToken  string
)

var bold  = color.New(color.Bold)
var green = color.New(color.FgGreen)
var red   = color.New(color.FgRed)
var cyan  = color.New(color.FgCyan)
var dim   = color.New(color.FgHiBlack)

var rootCmd = &cobra.Command{
	Use:   "psh",
	Short: "PhoneSSH — remote control your Android from the terminal",
	Long: `psh lets you control your Android phone from any terminal.

Quick start:
  1. Install PhoneSSH on your phone and tap "Start Service"
  2. Run: psh pair   (scan the QR code shown on your phone)
  3. Run: psh status

Examples:
  psh status
  psh ls /sdcard/DCIM
  psh pull /sdcard/DCIM/photo.jpg ./
  psh notifs
  psh sms list --unread
  psh apps launch spotify
  psh dnd on`,
	SilenceUsage: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&flagDevice, "device", "d", "", "device name (from psh devices)")
	rootCmd.PersistentFlags().StringVar(&flagHost, "host", "", "override phone IP/hostname")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port", 8765, "override port")
	rootCmd.PersistentFlags().StringVar(&flagToken, "token", "", "override auth token")

	rootCmd.AddCommand(pairCmd)
	rootCmd.AddCommand(devicesCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(batteryCmd)
	rootCmd.AddCommand(locationCmd)
	rootCmd.AddCommand(screenshotCmd)
	rootCmd.AddCommand(lsCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(findCmd)
	rootCmd.AddCommand(rmCmd)
	rootCmd.AddCommand(notifsCmd)
	rootCmd.AddCommand(smsCmd)
	rootCmd.AddCommand(appsCmd)
	rootCmd.AddCommand(volumeCmd)
	rootCmd.AddCommand(brightnessCmd)
	rootCmd.AddCommand(dndCmd)
	rootCmd.AddCommand(wifiCmd)
	rootCmd.AddCommand(clipboardCmd)
	rootCmd.AddCommand(shellCmd)
	rootCmd.AddCommand(openCmd)
	rootCmd.AddCommand(tapCmd)
	rootCmd.AddCommand(swipeCmd)
	rootCmd.AddCommand(typeCmd)
	rootCmd.AddCommand(keyCmd)
	rootCmd.AddCommand(aiCmd)
}

// getClient loads config and connects to the phone.
func getClient() (*client.Client, *client.Device, error) {
	// Override via flags
	if flagHost != "" {
		token := flagToken
		if token == "" {
			return nil, nil, fmt.Errorf("--host requires --token")
		}
		dev := &client.Device{
			Name:  "override",
			Host:  flagHost,
			Port:  flagPort,
			Token: token,
		}
		c, err := client.Connect(dev)
		return c, dev, err
	}

	cfg, err := client.LoadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("loading config: %w", err)
	}

	dev, err := cfg.GetDevice(flagDevice)
	if err != nil {
		return nil, nil, err
	}

	c, err := client.Connect(dev)
	return c, dev, err
}

// mustConnect is a helper that exits on error.
func mustConnect() (*client.Client, *client.Device) {
	c, dev, err := getClient()
	if err != nil {
		red.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
	return c, dev
}

// newCmd builds a CmdMsg with a unique ID.
func newCmd(cmd string, args []string, flags map[string]string) client.CmdMsg {
	return client.CmdMsg{
		Type:  "cmd",
		ID:    fmt.Sprintf("%d", time.Now().UnixNano()),
		Cmd:   cmd,
		Args:  args,
		Flags: flags,
	}
}

func die(format string, args ...interface{}) {
	red.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}
