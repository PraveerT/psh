package cmd

import (
	"fmt"
	"text/tabwriter"
	"os"

	"github.com/spf13/cobra"
)

var appsCmd = &cobra.Command{
	Use:   "apps",
	Short: "Manage apps on the phone",
	Long: `Manage installed apps on your phone.

Examples:
  psh apps list                  List user-installed apps
  psh apps list --system         Include system apps
  psh apps list --filter spotify Filter by name
  psh apps launch spotify        Open Spotify
  psh apps kill twitter          Kill Twitter in background
  psh apps info com.spotify.music  App details
  psh apps install /sdcard/app.apk Sideload an APK`,
}

var appsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed apps",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		flags := map[string]string{}
		if system, _ := cmd.Flags().GetBool("system"); system {
			flags["system"] = "true"
		}
		if filter, _ := cmd.Flags().GetString("filter"); filter != "" {
			flags["filter"] = filter
		}

		data, err := c.RunRaw(newCmd("apps", []string{"list"}, flags))
		if err != nil {
			return err
		}

		apps, ok := data["apps"].([]interface{})
		if !ok {
			return fmt.Errorf("unexpected response")
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "NAME\tPACKAGE\tSTATUS\n")
		for _, a := range apps {
			app, ok := a.(map[string]interface{})
			if !ok {
				continue
			}
			name := str(app["name"])
			pkg := str(app["package"])
			enabled := app["enabled"].(bool)
			status := "enabled"
			if !enabled {
				status = dim.Sprint("disabled")
			}
			fmt.Fprintf(w, "%s\t%s\t%s\n", name, dim.Sprint(pkg), status)
		}
		w.Flush()
		fmt.Printf("\n%v app(s)\n", data["count"])
		return nil
	},
}

var appsLaunchCmd = &cobra.Command{
	Use:   "launch <name-or-package>",
	Short: "Launch an app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		data, err := c.RunRaw(newCmd("apps", []string{"launch", args[0]}, nil))
		if err != nil {
			return err
		}
		green.Printf("Launched: %v\n", data["launched"])
		return nil
	},
}

var appsKillCmd = &cobra.Command{
	Use:   "kill <name-or-package>",
	Short: "Kill an app's background processes",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		data, err := c.RunRaw(newCmd("apps", []string{"kill", args[0]}, nil))
		if err != nil {
			return err
		}
		green.Printf("Killed: %v\n", data["killed"])
		if note, ok := data["note"].(string); ok {
			dim.Printf("Note: %s\n", note)
		}
		return nil
	},
}

var appsInfoCmd = &cobra.Command{
	Use:   "info <name-or-package>",
	Short: "Show app information",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		data, err := c.RunRaw(newCmd("apps", []string{"info", args[0]}, nil))
		if err != nil {
			return err
		}

		bold.Printf("%v\n", data["name"])
		fmt.Printf("Package:   %v\n", data["package"])
		fmt.Printf("Version:   %v (%v)\n", data["version_name"], data["version_code"])
		fmt.Printf("System:    %v\n", data["system"])
		fmt.Printf("Enabled:   %v\n", data["enabled"])
		fmt.Printf("APK:       %v\n", data["apk_path"])
		return nil
	},
}

var appsInstallCmd = &cobra.Command{
	Use:   "install <remote-apk-path>",
	Short: "Install an APK from the phone's storage",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		data, err := c.RunRaw(newCmd("apps", []string{"install", args[0]}, nil))
		if err != nil {
			return err
		}
		green.Printf("Installing: %v\n", data["installing"])
		if note, ok := data["note"].(string); ok {
			dim.Printf("%s\n", note)
		}
		return nil
	},
}

var appsUninstallCmd = &cobra.Command{
	Use:   "uninstall <package>",
	Short: "Uninstall an app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		data, err := c.RunRaw(newCmd("apps", []string{"uninstall", args[0]}, nil))
		if err != nil {
			return err
		}
		green.Printf("Uninstalling: %v\n", data["uninstalling"])
		return nil
	},
}

func init() {
	appsListCmd.Flags().Bool("system", false, "include system apps")
	appsListCmd.Flags().String("filter", "", "filter by name or package")

	appsCmd.AddCommand(appsListCmd)
	appsCmd.AddCommand(appsLaunchCmd)
	appsCmd.AddCommand(appsKillCmd)
	appsCmd.AddCommand(appsInfoCmd)
	appsCmd.AddCommand(appsInstallCmd)
	appsCmd.AddCommand(appsUninstallCmd)
}
