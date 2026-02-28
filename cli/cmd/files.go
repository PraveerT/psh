package cmd

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var lsCmd = &cobra.Command{
	Use:   "ls [remote-path]",
	Short: "List files on the phone",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		path := "/sdcard"
		if len(args) == 1 {
			path = args[0]
		}

		data, err := c.RunRaw(newCmd("ls", []string{path}, nil))
		if err != nil {
			return err
		}

		entries, ok := data["entries"].([]interface{})
		if !ok {
			return fmt.Errorf("unexpected response format")
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, e := range entries {
			entry, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			entryType := entry["type"].(string)
			name := entry["name"].(string)
			size := int64(entry["size"].(float64))
			modified := int64(entry["modified"].(float64))
			t := time.Unix(modified/1000, 0)

			typeChar := "-"
			nameStr := name
			if entryType == "dir" {
				typeChar = "d"
				nameStr = cyan.Sprint(name) + "/"
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				typeChar,
				formatSize(size),
				t.Format("Jan 02 15:04"),
				nameStr,
			)
		}
		w.Flush()
		return nil
	},
}

var findCmd = &cobra.Command{
	Use:   "find <pattern> [remote-path]",
	Short: "Search for files on the phone",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		cmdArgs := []string{args[0]}
		if len(args) == 2 {
			cmdArgs = append(cmdArgs, args[1])
		}

		data, err := c.RunRaw(newCmd("find", cmdArgs, nil))
		if err != nil {
			return err
		}

		matches, ok := data["matches"].([]interface{})
		if !ok {
			fmt.Println("No matches")
			return nil
		}

		for _, m := range matches {
			match, ok := m.(map[string]interface{})
			if !ok {
				continue
			}
			path := match["path"].(string)
			if match["type"].(string) == "dir" {
				fmt.Println(cyan.Sprint(path) + "/")
			} else {
				fmt.Printf("%s  %s\n", path, dim.Sprint(formatSize(int64(match["size"].(float64)))))
			}
		}
		fmt.Printf("\n%v match(es)\n", data["matches"].([]interface{}))
		return nil
	},
}

var pullCmd = &cobra.Command{
	Use:   "pull <remote-path> [local-dir]",
	Short: "Download a file from the phone",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, _ := mustConnect()
		defer c.Close()

		remotePath := args[0]
		localDir := "."
		if len(args) == 2 {
			localDir = args[1]
		}

		fmt.Printf("Downloading %s ...\n", remotePath)
		data, err := c.RunRaw(newCmd("pull", []string{remotePath}, nil))
		if err != nil {
			return err
		}

		content, ok := data["content"].(string)
		if !ok {
			return fmt.Errorf("no content in response")
		}
		fileBytes, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return fmt.Errorf("decoding file: %w", err)
		}

		filename := data["filename"].(string)
		localPath := filepath.Join(localDir, filename)

		// If localDir is a file path (has extension), use it directly
		if info, err := os.Stat(localDir); err == nil && !info.IsDir() {
			localPath = localDir
		} else if strings.HasSuffix(localDir, string(os.PathSeparator)) ||
			(!strings.Contains(filepath.Base(localDir), ".") && localDir != ".") {
			os.MkdirAll(localDir, 0755)
		}

		if err := os.WriteFile(localPath, fileBytes, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", localPath, err)
		}

		green.Printf("Saved: %s (%s)\n", localPath, formatSize(int64(len(fileBytes))))
		return nil
	},
}

var pushCmd = &cobra.Command{
	Use:   "push <local-file> <remote-path>",
	Short: "Upload a file to the phone",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		localPath := args[0]
		remotePath := args[1]

		fileBytes, err := os.ReadFile(localPath)
		if err != nil {
			return fmt.Errorf("reading %s: %w", localPath, err)
		}

		c, _ := mustConnect()
		defer c.Close()

		fmt.Printf("Uploading %s → %s ...\n", localPath, remotePath)
		payload := base64.StdEncoding.EncodeToString(fileBytes)
		msg := newCmd("push", []string{remotePath}, nil)
		msg.Payload = payload

		data, err := c.RunRaw(msg)
		if err != nil {
			return err
		}
		green.Printf("Uploaded %s bytes to %s\n", formatSize(int64(data["written"].(float64))), remotePath)
		return nil
	},
}

var rmCmd = &cobra.Command{
	Use:   "rm <remote-path>",
	Short: "Delete a file or directory on the phone",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")
		if !force {
			fmt.Printf("Delete %s on phone? [y/N] ", args[0])
			var confirm string
			fmt.Scanln(&confirm)
			if strings.ToLower(confirm) != "y" {
				fmt.Println("Aborted")
				return nil
			}
		}

		c, _ := mustConnect()
		defer c.Close()

		_, err := c.RunRaw(newCmd("rm", args, nil))
		if err != nil {
			return err
		}
		green.Printf("Deleted: %s\n", args[0])
		return nil
	},
}

func init() {
	rmCmd.Flags().BoolP("force", "f", false, "skip confirmation prompt")
}

func formatSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%d B", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	case bytes < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(bytes)/1024/1024)
	default:
		return fmt.Sprintf("%.2f GB", float64(bytes)/1024/1024/1024)
	}
}
