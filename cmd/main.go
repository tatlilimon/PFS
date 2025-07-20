package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/tatlilimon/PFS/internal/llm"
)

const correctedCmdFile = "/tmp/pfs_cmd"

// CommandInfo holds the information about the last executed command.
type CommandInfo struct {
	Command  string `json:"command"`
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
}

// showLoadingAnimation displays a simple animation until the done channel is closed.
func showLoadingAnimation(done <-chan struct{}) {

	animation := []string{"|", "/", "-", "\\"}
	i := 0
	for {
		select {
		case <-done:
			// Clear the loading animation line.
			fmt.Print("\r")
			return
		default:
			fmt.Printf("\rAsking the llm for your last failed command... %s", animation[i])
			i = (i + 1) % len(animation)
			time.Sleep(100 * time.Millisecond)
		}
	}
}

func main() {
	// Parse command-line arguments.
	verbose := flag.Bool("verbose", false, "Enable verbose output for debugging")
	flag.Parse()

	// Read and parse the last command's info from stdin.
	infoData, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to read from stdin: %v\n", err)
		os.Exit(1)
	}

	var info CommandInfo
	if err := json.Unmarshal(infoData, &info); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to parse command info from stdin: %v\n", err)
		os.Exit(1)
	}

	// Load configuration from ~/.pfs.env
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get user home directory: %v\n", err)
		os.Exit(1)
	}
	if err := godotenv.Load(filepath.Join(home, ".pfs.env")); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load .pfs.env file from home directory: %v\n", err)
		os.Exit(1)
	}

	// Get the configured LLM provider and check connection.
	provider, err := llm.NewOllamaProvider()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✅ Connected to Ollama model: %s\n", provider.ModelName())

	// Get the correction from the LLM with a loading animation.
	ctx := context.Background()
	var correction *llm.Correction
	done := make(chan struct{})
	go func() {
		defer close(done)
		correction, err = provider.GetCorrection(ctx, info.Command, info.Output, info.ExitCode, *verbose)
	}()

	showLoadingAnimation(done)
	<-done

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to get correction from LLM: %v\n", err)
		os.Exit(1)
	}

	// Display the correction and ask for confirmation.
	if correction.CorrectedCommand == "" {
		fmt.Println("\n🧠 Explanation: The LLM did not return a corrected command.")
		os.Exit(0)
	}
	// Validate the corrected command.
	if !isCommandAvailable(correction.CorrectedCommand) {
		if *verbose {
			fmt.Fprintf(os.Stderr, "Error: Corrected command is not valid or not in PATH: %s\n", correction.CorrectedCommand)
		}
		fmt.Println("\n🧠 Explanation: The LLM returned a command that is not valid or not in your PATH. No action will be taken.")
		os.Exit(1)
	}

	fmt.Printf("\n🧠 Explanation: %s\n", correction.Explanation)
	fmt.Printf("🔧 Corrected: \033[1;32m%s\033[0m\n\n", correction.CorrectedCommand)

	if correction.CorrectedCommand == "" {
		os.Exit(0)
	}

	fmt.Print("> Execute this command? (y/n) ")

	// Open /dev/tty for interactive input, separate from stdin
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to open tty for user input: %v\n", err)
		os.Exit(1)
	}
	defer tty.Close()

	scanner := bufio.NewScanner(tty)
	if scanner.Scan() {
		answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if answer == "y" || answer == "yes" {
			// 8. Write the command to the temp file for the shell wrapper to execute.
			err := os.WriteFile(correctedCmdFile, []byte(correction.CorrectedCommand), 0644)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Failed to write corrected command to temp file: %v\n", err)
				os.Exit(1)
			}
			// The shell wrapper will now pick up this file.
		} else {
			fmt.Println("Aborted.")
		}
	} else if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to read user input: %v\n", err)
		os.Exit(1)
	} else {
		// Nothing was entered, treat as "no"
		fmt.Println("Aborted.")
	}
}

// isCommandAvailable checks if a command is in the system's PATH.
func isCommandAvailable(name string) bool {
	cmd := strings.Fields(name)[0]
	_, err := exec.LookPath(cmd)
	return err == nil
}
