# lazystatus

```
██╗      █████╗ ███████╗██╗   ██╗███████╗████████╗ █████╗ ████████╗██╗   ██╗███████╗
██║     ██╔══██╗╚══███╔╝╚██╗ ██╔╝██╔════╝╚══██╔══╝██╔══██╗╚══██╔══╝██║   ██║██╔════╝
██║     ███████║  ███╔╝  ╚████╔╝ ███████╗   ██║   ███████║   ██║   ██║   ██║███████╗
██║     ██╔══██║ ███╔╝    ╚██╔╝  ╚════██║   ██║   ██╔══██║   ██║   ██║   ██║╚════██║
███████╗██║  ██║███████╗   ██║   ███████║   ██║   ██║  ██║   ██║   ╚██████╔╝███████║
╚══════╝╚═╝  ╚═╝╚══════╝   ╚═╝   ╚══════╝   ╚═╝   ╚═╝  ╚═╝   ╚═╝    ╚═════╝ ╚══════╝
```

<div align="center">

### A modern TUI for monitoring status pages

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org/)
[![MIT License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)](LICENSE)
[![Bubble Tea](https://img.shields.io/badge/Built%20with-Bubble%20Tea-FF1493?style=for-the-badge&logo=terminal&logoColor=white)](https://github.com/charmbracelet/bubbletea)

</div>

---

Monitor multiple status pages (GitHub, Atlassian, Cloudflare, etc.) in a beautiful terminal interface. Built with [Charm's Bubble Tea](https://github.com/charmbracelet/bubbletea) framework for smooth, flicker-free rendering.

## Features

- **🔍 Smart Status Detection** - Auto-detects Statuspage.io JSON API or falls back to HTML parsing
- **🎨 Color-Coded Status** - Green (operational), Blue (maintenance), Yellow (degraded), Red (disruption)
- **⚡ Auto-Refresh** - Configurable per-service refresh intervals (default: 30s)
- **📊 Detailed View** - Incidents, maintenance windows, timestamps, and resolution status
- **⌨️ Vim-Style Navigation** - Efficient keyboard shortcuts for power users
- **💾 Persistent Config** - Services saved to `~/.lazystatus/config.json`
- **🔄 Real-Time Updates** - Live countdown timers and status changes
- **🌐 Proxy Support** - Respects `http_proxy` environment variables (Zscaler compatible)

## Installation

### Homebrew (macOS)

```bash
# Add the tap and install
brew tap jakeasaurus/tap
brew install lazystatus

# Or in one line
brew install jakeasaurus/tap/lazystatus
```

### Build from Source

```bash
git clone https://github.com/jakeasaurus/lazystatus.git
cd lazystatus
go build -o lazystatus
./lazystatus
```

### Quick Install

```bash
go install github.com/jakeasaurus/lazystatus@latest
```

## Usage

```bash
# Start the TUI
lazystatus

# Show version
lazystatus --version

# Show help
lazystatus --help
```

## Key Bindings

### Navigation
- `j` or `↓` - Move down
- `k` or `↑` - Move up
- `g` or `Home` - Go to top
- `G` or `End` - Go to bottom

### Service Actions
- `a` - Add new service
- `e` - Edit selected service
- `d` - Delete selected service
- `r` - Refresh all services

### Other
- `?` - Show/hide help
- `q` or `Ctrl+C` - Quit

### Input Mode
- `Tab` - Next field
- `Shift+Tab` - Previous field
- `Enter` - Submit
- `Esc` - Cancel

## Configuration

Services are stored in `~/.lazystatus/config.json`:

```json
{
  "services": [
    {
      "name": "GitHub",
      "url": "https://www.githubstatus.com",
      "refresh_interval": 30,
      "last_checked": "2025-10-20T15:20:00Z",
      "current_status": "operational"
    },
    {
      "name": "Cloudflare",
      "url": "https://www.cloudflarestatus.com",
      "refresh_interval": 60,
      "last_checked": "2025-10-20T15:20:00Z",
      "current_status": "operational"
    }
  ],
  "settings": {
    "default_refresh_interval": 30
  }
}
```

## Supported Status Pages

### Auto-Detection
- **Statuspage.io** - Automatically detects JSON API at `/api/v2/summary.json`
- **GitHub Status** - https://www.githubstatus.com
- **Atlassian Status** - https://status.atlassian.com
- **Cloudflare Status** - https://www.cloudflarestatus.com

### HTML Fallback
For non-Statuspage.io sites, lazystatus uses keyword detection:
- "operational" → Operational
- "degraded" / "partial outage" → Degraded Performance
- "major outage" / "major disruption" → Major Disruption
- "maintenance" / "scheduled" → Planned Maintenance

## Status Color Legend

- 🟢 **Green** (#04B575) - Operational
- 🔵 **Blue** (#5FAFFF) - Planned Maintenance
- 🟡 **Yellow** (#FFAF5F) - Degraded Performance
- 🔴 **Red** (#FF5F87) - Major Disruption / Connection Error

## Adding a Service

1. Press `a` to open the add service dialog
2. Enter:
   - **Name**: Display name (e.g., "GitHub")
   - **URL**: Status page URL (e.g., "https://www.githubstatus.com")
   - **Interval**: Refresh interval in seconds (5-86400)
3. Press `Enter` to save
4. Service will be fetched immediately

## Editing a Service

1. Navigate to the service with `j`/`k`
2. Press `e` to edit
3. Modify fields with `Tab` / `Shift+Tab`
4. Press `Enter` to save

## Deleting a Service

1. Navigate to the service with `j`/`k`
2. Press `d` to delete
3. Confirm with `Enter` or cancel with `Esc`

## Proxy Configuration

lazystatus respects standard HTTP proxy environment variables:

```bash
export http_proxy=http://proxy.example.com:8080
export https_proxy=http://proxy.example.com:8080
lazystatus
```

This works with corporate proxies like Zscaler.

## Development

### Requirements
- Go 1.21 or later
- Terminal with 24-bit color support (recommended)

### Dependencies
- [Bubble Tea](https://github.com/charmbracelet/bubbletea) - TUI framework
- [Bubbles](https://github.com/charmbracelet/bubbles) - TUI components
- [Lip Gloss](https://github.com/charmbracelet/lipgloss) - Styling
- [golang.org/x/net/html](https://pkg.go.dev/golang.org/x/net/html) - HTML parsing

### Building

```bash
go mod tidy
go build -o lazystatus
```

### Testing

Test with real status pages:

```bash
./lazystatus
# Press 'a' to add:
# - GitHub: https://www.githubstatus.com
# - Atlassian: https://status.atlassian.com
# - Cloudflare: https://www.cloudflarestatus.com
```

## Architecture

- `main.go` - CLI entrypoint
- `status.go` - Domain model and service manager with JSON persistence
- `app.go` - Bubble Tea model with TUI logic
- `internal/fetch/fetch.go` - HTTP client with Statuspage.io JSON parser and HTML fallback

## Why lazystatus?

- **Fast** - Minimal overhead, instant startup
- **Simple** - No complex configuration
- **Beautiful** - Modern TUI with smooth animations
- **Reliable** - Robust parsing with fallbacks
- **Portable** - Single binary, no dependencies

## Future Enhancements

- [ ] Desktop notifications on status changes (macOS)
- [ ] Sorting/filtering by status severity
- [ ] Export incidents to JSON/CSV
- [ ] Import services from file
- [ ] Per-service pause/resume

## Contributing

Contributions welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test thoroughly
5. Submit a pull request

## License

MIT License - See LICENSE file for details.

## Credits

Inspired by [lazygit](https://github.com/jesseduffield/lazygit) and built with ❤️ using [Charm](https://charm.sh) tools.

---

<div align="center">

**Made with ❤️ and Go**

</div>
