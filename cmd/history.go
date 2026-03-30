package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Manage session history",
}

var historyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all history sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := historyDir()
		if err != nil {
			return err
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("no history found")
				return nil
			}
			return err
		}

		// Collect and sort by modification time (newest last).
		type entry struct {
			name    string
			modTime time.Time
			lines   int
		}
		var sessions []entry
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			path := filepath.Join(dir, e.Name())
			n, _ := countLines(path)
			sessions = append(sessions, entry{
				name:    e.Name(),
				modTime: info.ModTime(),
				lines:   n,
			})
		}
		sort.Slice(sessions, func(i, j int) bool {
			return sessions[i].modTime.Before(sessions[j].modTime)
		})

		if len(sessions) == 0 {
			fmt.Println("no history found")
			return nil
		}

		for _, s := range sessions {
			short := s.name
			if len(short) > 12 {
				short = short[:12]
			}
			fmt.Printf("%s  %s  (%d commands)\n",
				s.modTime.Format("2006-01-02 15:04:05"),
				short,
				s.lines,
			)
		}
		return nil
	},
}

var historyShowCmd = &cobra.Command{
	Use:   "show <hash>",
	Short: "Print commands from a history session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := historyDir()
		if err != nil {
			return err
		}

		hash := args[0]
		path := filepath.Join(dir, hash)

		// Support prefix matching so the user can pass a short prefix.
		if _, err := os.Stat(path); os.IsNotExist(err) {
			match, err := findByPrefix(dir, hash)
			if err != nil {
				return fmt.Errorf("history: %s: not found", hash)
			}
			path = match
		}

		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("history: %w", err)
		}
		fmt.Printf("session: %s\n", filepath.Base(path))
		fmt.Printf("time:    %s\n\n", info.ModTime().Format("2006-01-02 15:04:05"))

		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("history: %w", err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		i := 1
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			fmt.Printf("%4d  %s\n", i, line)
			i++
		}
		return scanner.Err()
	},
}

// historyDir returns ~/.memsh/history.
func historyDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".memsh", "history"), nil
}

// countLines returns the number of non-empty lines in a file.
func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) != "" {
			n++
		}
	}
	return n, scanner.Err()
}

// findByPrefix looks for the unique file in dir whose name starts with prefix.
func findByPrefix(dir, prefix string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var matches []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			matches = append(matches, filepath.Join(dir, e.Name()))
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return "", fmt.Errorf("no unique match for %q", prefix)
}

func init() {
	historyCmd.AddCommand(historyListCmd, historyShowCmd)
	rootCmd.AddCommand(historyCmd)
}
