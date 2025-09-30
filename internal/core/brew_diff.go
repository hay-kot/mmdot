package core

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"

	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
)

type DiffResult struct {
	Present []string // Present on machine
	Absent  []string // Absent from machine
	Extra   []string // Present in config, Absent from machine
}

// Diff returns a comparison between the brews in the Config and those installed on the machine.
// It categorizes brews into:
// - Present: Items in config and installed on the machine
// - Absent: Items in config but not installed on the machine
// - Extra: Items installed on the machine but not in the config (drift detection)
func (c *Brews) Diff() (*DiffResult, error) {
	// Get the list of brews installed on the machine with spinner UI
	installedBrews := getInstalledBrews()

	// Initialize the result structure
	result := &DiffResult{
		Present: []string{},
		Absent:  []string{},
		Extra:   []string{},
	}

	// Create a map for O(1) lookup of installed brews
	installedMap := make(map[string]bool)
	for _, brew := range installedBrews {
		installedMap[brew] = true
	}

	// Create a map for O(1) lookup of configured brews (for tracking extras later)
	configMap := make(map[string]bool)

	// Check each brew in the config against the installed brews
	for _, brew := range slices.Concat(c.Brews, c.Casks) {
		// Add to config map for tracking
		configMap[brew] = true

		// Check if the brew is installed
		if installedMap[brew] {
			result.Present = append(result.Present, brew)
		} else {
			result.Absent = append(result.Absent, brew)
		}
	}

	// Find extras (installed but not in config)
	for _, brew := range installedBrews {
		if !configMap[brew] {
			result.Extra = append(result.Extra, brew)
		}
	}

	return result, nil
}

var spinnerStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("10")) // Green

func getInstalledBrews() []string {
	var brews, casks []string
	var brewsErr, casksErr error

	// Create an action function that will run both commands in parallel
	action := func() {
		var wg sync.WaitGroup
		wg.Add(2)

		// Get brews in a goroutine
		go func() {
			defer wg.Done()
			cmd := exec.Command("brew", "list", "--full-name", "--installed-on-request")
			output, err := cmd.Output()
			brewsErr = err
			if err == nil {
				brewsOutput := strings.Split(strings.TrimSpace(string(output)), "\n")
				for _, name := range brewsOutput {
					if name != "" {
						brews = append(brews, name)
					}
				}
			}
		}()

		// Get casks in a goroutine
		go func() {
			defer wg.Done()
			cmd := exec.Command("brew", "list", "--casks")
			output, err := cmd.Output()
			casksErr = err
			if err == nil {
				casksOutput := strings.Split(strings.TrimSpace(string(output)), "\n")
				for _, name := range casksOutput {
					if name != "" {
						casks = append(casks, name)
					}
				}
			}
		}()

		// Wait for both goroutines to complete
		wg.Wait()
	}

	// Run the action with a spinner
	spin := spinner.New().
		Type(spinner.Line).
		Style(spinnerStyle).
		Title(" Fetching installed brews and casks").
		Action(action)

	if err := spin.Run(); err != nil {
		fmt.Printf("Error with spinner: %v\n", err)
	}

	// Handle command errors
	if brewsErr != nil {
		fmt.Printf("Error getting installed brews: %v\n", brewsErr)
	}
	if casksErr != nil {
		fmt.Printf("Error getting installed casks: %v\n", casksErr)
	}

	// Combine results
	result := append(brews, casks...)
	return result
}
