package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

const version = "0.1.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Printf("lazystatus v%s\n", version)
			return
		case "--help", "-h":
			printHelp()
			return
		}
	}

	sm, err := NewServiceManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing service manager: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(
		initialModel(sm),
		tea.WithAltScreen(),
	)

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	sm.Save()
}

func printHelp() {
	fmt.Println("lazystatus - A TUI for monitoring status pages")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  lazystatus                 Start the TUI")
	fmt.Println("  lazystatus --version       Show version")
	fmt.Println("  lazystatus --help          Show this help")
	fmt.Println("")
	fmt.Println("Key bindings (once in TUI):")
	fmt.Println("Navigation:")
	fmt.Println("  j/↓        Move down")
	fmt.Println("  k/↑        Move up")
	fmt.Println("  g/Home     Go to top")
	fmt.Println("  G/End      Go to bottom")
	fmt.Println("")
	fmt.Println("Service actions:")
	fmt.Println("  a          Add new service")
	fmt.Println("  e          Edit service")
	fmt.Println("  d          Delete service")
	fmt.Println("  Enter      Refresh selected service")
	fmt.Println("  r          Refresh all services")
	fmt.Println("")
	fmt.Println("Other:")
	fmt.Println("  ?          Show/hide help")
	fmt.Println("  q/Ctrl+C   Quit")
	fmt.Println("")
	fmt.Println("Input mode keys:")
	fmt.Println("  Tab        Next field")
	fmt.Println("  Shift+Tab  Previous field")
	fmt.Println("  Enter      Submit")
	fmt.Println("  Esc        Cancel")
	fmt.Println("")
	fmt.Println("Configuration:")
	fmt.Println("  Config file: ~/.lazystatus/config.json")
	fmt.Println("")
	fmt.Println("🎭 Powered by Charm - https://charm.sh")
}
