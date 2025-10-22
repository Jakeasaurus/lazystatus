package main

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jakeasaurus/lazystatus/internal/fetch"
)

type InputMode int

const (
	ModeNormal InputMode = iota
	ModeAdd
	ModeEdit
	ModeHelp
	ModeConfirm
)

var (
	appStyle = lipgloss.NewStyle().Padding(0, 1)

	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	serviceListStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#874BFD")).
				Padding(0, 1)

	detailsStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#F25D94")).
			Padding(0, 1)

	commandStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FF7CCB")).
			Padding(0, 1)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#626262"))

	statusMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575"))
)

type tickMsg time.Time

type refreshMsg struct {
	Index int
	fetchResult *fetch.Result
	Err   error
}

type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	Add       key.Binding
	Edit      key.Binding
	Delete    key.Binding
	Open      key.Binding
	Refresh     key.Binding
	RefreshAll  key.Binding
	Help        key.Binding
	Quit        key.Binding
	Enter       key.Binding
	Escape    key.Binding
	Home      key.Binding
	End       key.Binding
	Tab       key.Binding
	ShiftTab  key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("↑/k", "move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("↓/j", "move down"),
	),
	Add: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "add service"),
	),
	Edit: key.NewBinding(
		key.WithKeys("e"),
		key.WithHelp("e", "edit service"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "delete service"),
	),
	Open: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "open in browser"),
	),
	Refresh: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "refresh selected"),
	),
	RefreshAll: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("r", "refresh all"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "quit"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "confirm"),
	),
	Escape: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("esc", "cancel"),
	),
	Home: key.NewBinding(
		key.WithKeys("g", "home"),
		key.WithHelp("g/home", "go to top"),
	),
	End: key.NewBinding(
		key.WithKeys("G", "end"),
		key.WithHelp("G/end", "go to bottom"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "next field"),
	),
	ShiftTab: key.NewBinding(
		key.WithKeys("shift+tab"),
		key.WithHelp("shift+tab", "previous field"),
	),
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Add, k.Edit, k.Delete, k.Open, k.Refresh, k.RefreshAll, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Home, k.End},
		{k.Add, k.Edit, k.Delete, k.Open},
		{k.Refresh, k.RefreshAll, k.Help, k.Quit},
	}
}

type Model struct {
	manager       *ServiceManager
	fetchClient   *fetch.Client
	selected      int
	sortedIndices []int // Maps display position to actual service index
	mode          InputMode
	nameInput     textinput.Model
	urlInput      textinput.Model
	intervalInput textinput.Model
	focusedInput  int
	viewport      viewport.Model
	help          help.Model
	statusMsg     string
	width         int
	height        int
	deleteTarget  int
}

func initialModel(sm *ServiceManager) Model {
	nameInput := textinput.New()
	nameInput.Placeholder = "Service Name"
	nameInput.CharLimit = 100
	nameInput.Width = 50

	urlInput := textinput.New()
	urlInput.Placeholder = "https://status.example.com"
	urlInput.CharLimit = 500
	urlInput.Width = 50

	intervalInput := textinput.New()
	intervalInput.Placeholder = "30"
	intervalInput.CharLimit = 6
	intervalInput.Width = 20

	vp := viewport.New(80, 20)

	return Model{
		manager:       sm,
		fetchClient:   fetch.NewClient(),
		selected:      0,
		mode:          ModeNormal,
		nameInput:     nameInput,
		urlInput:      urlInput,
		intervalInput: intervalInput,
		viewport:      vp,
		help:          help.New(),
	}
}

func (m Model) Init() tea.Cmd {
	// Initialize sorted indices
	services := m.manager.List()
	m.sortedIndices = make([]int, len(services))
	for i := range services {
		m.sortedIndices[i] = i
	}
	m.updateSortedIndices()
	
	return tea.Batch(
		tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return tickMsg(t)
		}),
		m.refreshAllCmd(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		
		listWidth := msg.Width*3/5 - 4
		detailsWidth := msg.Width*2/5 - 6
		if detailsWidth < 30 {
			detailsWidth = 30
		}
		
		vpHeight := msg.Height - 12
		if vpHeight < 5 {
			vpHeight = 5
		}
		
		m.viewport.Width = detailsWidth
		m.viewport.Height = vpHeight
		m.help.Width = msg.Width
		
		serviceListStyle = serviceListStyle.Width(listWidth)
		detailsStyle = detailsStyle.Width(detailsWidth)
		
		return m, nil

	case tickMsg:
		now := time.Now()
		services := m.manager.List()
		for i, svc := range services {
			if !svc.InFlight && now.After(svc.NextRefreshAt) {
				cmds = append(cmds, m.refreshServiceCmd(i))
			}
		}
		cmds = append(cmds, tea.Tick(time.Second, func(t time.Time) tea.Msg {
			return tickMsg(t)
		}))
		return m, tea.Batch(cmds...)

	case statusMsg:
		m.statusMsg = string(msg)
		return m, nil

	case refreshMsg:
		if msg.Err != nil {
			m.manager.UpdateStatus(msg.Index, StatusConnectionError, nil, nil, "", msg.Err.Error())
		} else {
			incidents := convertIncidents(msg.fetchResult.Incidents)
			maintenances := convertMaintenances(msg.fetchResult.Maintenances)
			level := convertStatusLevel(msg.fetchResult.Level)
			m.manager.UpdateStatus(msg.Index, level, incidents, maintenances, msg.fetchResult.ParseNote, "")
		}
		m.manager.Save()
		m.updateSortedIndices()
		m.viewport.SetContent(m.renderDetails())
		return m, nil

	case tea.KeyMsg:
		if m.mode == ModeHelp {
			if msg.String() == "?" || msg.String() == "esc" || msg.String() == "q" {
				m.mode = ModeNormal
			}
			return m, nil
		}

		if m.mode != ModeNormal {
			return m.handleInputMode(msg)
		}

		switch {
		case key.Matches(msg, keys.Quit):
			m.manager.Save()
			return m, tea.Quit

		case key.Matches(msg, keys.Up):
			if m.selected > 0 {
				m.selected--
			}
			m.viewport.SetContent(m.renderDetails())

		case key.Matches(msg, keys.Down):
			if m.selected < len(m.sortedIndices)-1 {
				m.selected++
			}
			m.viewport.SetContent(m.renderDetails())

		case key.Matches(msg, keys.Home):
			m.selected = 0
			m.viewport.SetContent(m.renderDetails())

		case key.Matches(msg, keys.End):
			if len(m.sortedIndices) > 0 {
				m.selected = len(m.sortedIndices) - 1
			}
			m.viewport.SetContent(m.renderDetails())

		case key.Matches(msg, keys.Add):
			m.mode = ModeAdd
			m.focusedInput = 0
			m.nameInput.SetValue("")
			m.urlInput.SetValue("")
			m.intervalInput.SetValue(fmt.Sprintf("%d", m.manager.GetDefaultInterval()))
			m.nameInput.Focus()
			m.urlInput.Blur()
			m.intervalInput.Blur()

		case key.Matches(msg, keys.Edit):
			if m.selected < len(m.sortedIndices) {
				actualIndex := m.sortedIndices[m.selected]
				services := m.manager.List()
				if actualIndex < len(services) {
					m.mode = ModeEdit
					m.focusedInput = 0
					svc := services[actualIndex]
					m.nameInput.SetValue(svc.Config.Name)
					m.urlInput.SetValue(svc.Config.URL)
					m.intervalInput.SetValue(fmt.Sprintf("%d", svc.Config.RefreshIntervalSeconds))
					m.nameInput.Focus()
					m.urlInput.Blur()
					m.intervalInput.Blur()
				}
			}

		case key.Matches(msg, keys.Delete):
			if m.selected < len(m.sortedIndices) {
				actualIndex := m.sortedIndices[m.selected]
				services := m.manager.List()
				if actualIndex < len(services) {
					m.mode = ModeConfirm
					m.deleteTarget = actualIndex
				}
			}

		case key.Matches(msg, keys.Open):
			if m.selected < len(m.sortedIndices) {
				actualIndex := m.sortedIndices[m.selected]
				services := m.manager.List()
				if actualIndex < len(services) {
					return m, m.openURLCmd(services[actualIndex].Config.URL)
				}
			}

		case key.Matches(msg, keys.Refresh):
			if m.selected < len(m.sortedIndices) {
				actualIndex := m.sortedIndices[m.selected]
				services := m.manager.List()
				if actualIndex < len(services) {
					m.statusMsg = fmt.Sprintf("Refreshing %s...", services[actualIndex].Config.Name)
					return m, m.refreshServiceCmd(actualIndex)
				}
			}

		case key.Matches(msg, keys.RefreshAll):
			m.statusMsg = "Refreshing all services..."
			return m, m.refreshAllCmd()

		case key.Matches(msg, keys.Help):
			m.mode = ModeHelp
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleInputMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.mode == ModeConfirm {
		switch msg.String() {
		case "enter":
			services := m.manager.List()
			if m.deleteTarget >= 0 && m.deleteTarget < len(services) {
				// Get the name for confirmation message
				deletedName := services[m.deleteTarget].Config.Name
				
				// Delete the service
				m.manager.Remove(m.deleteTarget)
				m.manager.Save()
				
				// Rebuild sorted indices after deletion
				m.updateSortedIndices()
				
				// Adjust selection to stay in bounds
				if m.selected >= len(m.sortedIndices) && len(m.sortedIndices) > 0 {
					m.selected = len(m.sortedIndices) - 1
				} else if len(m.sortedIndices) == 0 {
					m.selected = 0
				}
				
				m.statusMsg = fmt.Sprintf("Deleted: %s", deletedName)
			}
			m.mode = ModeNormal
			m.viewport.SetContent(m.renderDetails())
		case "esc":
			m.mode = ModeNormal
		}
		return m, nil
	}

	switch msg.String() {
	case "enter":
		if err := m.submitService(); err != nil {
			m.statusMsg = "Error: " + err.Error()
		} else {
			var idx int
			if m.mode == ModeAdd {
				idx = len(m.manager.List()) - 1
				// Rebuild sorted indices after adding
				m.updateSortedIndices()
				// Find the new service in sorted view
				for i, sortedIdx := range m.sortedIndices {
					if sortedIdx == idx {
						m.selected = i
						break
					}
				}
			} else {
				idx = m.sortedIndices[m.selected]
				// Rebuild sorted indices after editing (status might have changed)
				m.updateSortedIndices()
			}
			m.statusMsg = "Service saved"
			m.mode = ModeNormal
			m.viewport.SetContent(m.renderDetails())
			return m, m.refreshServiceCmd(idx)
		}
		return m, nil

	case "esc":
		m.mode = ModeNormal
		return m, nil

	case "tab":
		m.focusedInput = (m.focusedInput + 1) % 3
		m.updateInputFocus()
		return m, nil

	case "shift+tab":
		m.focusedInput = (m.focusedInput + 2) % 3
		m.updateInputFocus()
		return m, nil
	}

	switch m.focusedInput {
	case 0:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case 1:
		m.urlInput, cmd = m.urlInput.Update(msg)
	case 2:
		m.intervalInput, cmd = m.intervalInput.Update(msg)
	}

	return m, cmd
}

func (m *Model) updateInputFocus() {
	m.nameInput.Blur()
	m.urlInput.Blur()
	m.intervalInput.Blur()

	switch m.focusedInput {
	case 0:
		m.nameInput.Focus()
	case 1:
		m.urlInput.Focus()
	case 2:
		m.intervalInput.Focus()
	}
}

func (m *Model) submitService() error {
	name := strings.TrimSpace(m.nameInput.Value())
	urlStr := strings.TrimSpace(m.urlInput.Value())
	intervalStr := strings.TrimSpace(m.intervalInput.Value())

	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	if urlStr == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	if _, err := url.Parse(urlStr); err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		return fmt.Errorf("URL must start with http:// or https://")
	}

	interval, err := strconv.Atoi(intervalStr)
	if err != nil {
		return fmt.Errorf("invalid interval: must be a number")
	}

	if interval < 5 || interval > 86400 {
		return fmt.Errorf("interval must be between 5 and 86400 seconds")
	}

	cfg := ServiceConfig{
		Name:                   name,
		URL:                    urlStr,
		RefreshIntervalSeconds: interval,
	}

	if m.mode == ModeAdd {
		return m.manager.Add(cfg)
	} else if m.mode == ModeEdit {
		// Use actual index, not sorted index
		actualIdx := m.sortedIndices[m.selected]
		return m.manager.Update(actualIdx, cfg)
	}

	return nil
}

func (m Model) View() string {
	if m.mode == ModeHelp {
		return m.renderHelp()
	}

	services := m.manager.List()
	
	listContent := m.renderServiceList(services)
	detailsContent := detailsStyle.Render(m.viewport.View())
	commandContent := m.renderCommandWindow()
	statusBar := m.renderStatusBar(services)

	var mainContent string
	if m.height < 20 {
		mainContent = lipgloss.NewStyle().
			Render(listContent)
	} else {
		mainContent = lipgloss.NewStyle().
			Render(
				lipgloss.JoinHorizontal(
					lipgloss.Top,
					serviceListStyle.Render(listContent),
					" ",
					detailsContent,
				),
			)
	}

	helpText := "? for help • q to quit"
	if m.mode != ModeNormal {
		helpText = "Enter: save • ESC: cancel"
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		commandContent,
		mainContent,
		statusBar,
		helpStyle.Render(helpText),
	)
}

func (m Model) renderHelp() string {
	return lipgloss.NewStyle().
		Margin(1, 2).
		Render(
			titleStyle.Render("🔍 lazystatus - Help") + "\n\n" +
				m.help.View(keys) + "\n\n" +
				helpStyle.Render("Press ? to close help"),
		)
}

func (m *Model) renderServiceList(services []ServiceState) string {
	if len(services) == 0 {
		return helpStyle.Render("No services configured. Press 'a' to add one.")
	}

	listWidth := m.width*3/5 - 4
	if listWidth < 30 {
		listWidth = 30
	}

	// Use existing sorted indices to render in the right order
	var lines []string
	var addedOperationalSeparator bool
	var addedMaintenanceSeparator bool
	var addedDegradedSeparator bool
	var addedCriticalSeparator bool
	for i, origIdx := range m.sortedIndices {
		if origIdx >= len(services) {
			continue
		}
		svc := services[origIdx]
		
		// Add separator before first major disruption/critical service
		if !addedCriticalSeparator && (svc.StatusLevel == StatusMajorDisruption || 
			svc.StatusLevel == StatusConnectionError || svc.StatusLevel == StatusParseError) {
			separator := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF5F87")). // Red for critical
				Render("🚨 Critical Issues " + strings.Repeat("─", listWidth-20))
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, separator)
			addedCriticalSeparator = true
		}
		
		// Add separator before first degraded service
		if !addedDegradedSeparator && svc.StatusLevel == StatusDegraded {
			separator := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFAF5F")). // Orange for degraded
				Render("⚠️  Degraded Performance " + strings.Repeat("─", listWidth-27))
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, separator)
			addedDegradedSeparator = true
		}
		
		// Add separator before first planned maintenance service
		if !addedMaintenanceSeparator && svc.StatusLevel == StatusPlannedMaintenance {
			separator := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#5FAFFF")). // Blue for maintenance
				Render("🔧 Planned Maintenance " + strings.Repeat("─", listWidth-25))
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, separator)
			addedMaintenanceSeparator = true
		}
		
		// Add separator before first operational service
		if !addedOperationalSeparator && svc.StatusLevel == StatusOperational {
			if i > 0 { // Only add if there are non-operational services above
				separator := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#04B575")). // Green for operational
					Render(strings.Repeat("─", listWidth-6) + " ✓ All Systems Operational")
				lines = append(lines, "")
				lines = append(lines, separator)
				addedOperationalSeparator = true
			}
		}
		
		statusDot := lipgloss.NewStyle().
			Foreground(lipgloss.Color(svc.StatusLevel.Color())).
			Render("●")

		name := svc.Config.Name
		if i == m.selected {
			name = lipgloss.NewStyle().Bold(true).Render(name)
		}

		status := svc.StatusLevel.String()
		
		var countdown string
		if svc.InFlight {
			countdown = helpStyle.Render("Refreshing...")
		} else {
			until := time.Until(svc.NextRefreshAt).Round(time.Second)
			if until < 0 {
				until = 0
			}
			countdown = helpStyle.Render(fmt.Sprintf("Next: %ds", int(until.Seconds())))
		}

		line := fmt.Sprintf("%s %s\n  %s • %s", statusDot, name, status, countdown)
		if i == m.selected {
			selectorStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#FF79C6")).
				Background(lipgloss.Color("#282A36")).
				Padding(0, 1).
				Width(listWidth - 6)
			lines = append(lines, selectorStyle.Render(line))
		} else {
			plainStyle := lipgloss.NewStyle().
				Padding(0, 1)
			lines = append(lines, plainStyle.Render(line))
		}
	}

	return strings.Join(lines, "\n\n")
}

func (m Model) renderDetails() string {
	services := m.manager.List()
	if len(services) == 0 || m.selected >= len(m.sortedIndices) {
		return helpStyle.Render("No service selected")
	}
	
	// Get actual service index from sorted position
	actualIndex := m.sortedIndices[m.selected]
	if actualIndex >= len(services) {
		return helpStyle.Render("No service selected")
	}

	svc := services[actualIndex]
	
	var lines []string
	lines = append(lines, lipgloss.NewStyle().Bold(true).Render("📊 Service Details"))
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("Name: %s", svc.Config.Name))
	lines = append(lines, fmt.Sprintf("URL: %s", svc.Config.URL))
	
	statusColor := lipgloss.NewStyle().Foreground(lipgloss.Color(svc.StatusLevel.Color()))
	lines = append(lines, fmt.Sprintf("Status: %s", statusColor.Render(svc.StatusLevel.String())))
	
	if !svc.Config.LastChecked.IsZero() {
		lines = append(lines, fmt.Sprintf("Last Checked: %s", svc.Config.LastChecked.Format("15:04:05")))
	}
	
	lines = append(lines, fmt.Sprintf("Refresh Interval: %ds", svc.Config.RefreshIntervalSeconds))

	if svc.LastError != "" {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F87")).Render("Error: "+svc.LastError))
	}

	if len(svc.Incidents) > 0 {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render("🚨 Recent Incidents:"))
		for _, inc := range svc.Incidents {
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("• %s", inc.Title))
			lines = append(lines, fmt.Sprintf("  Impact: %s", inc.Impact))
			lines = append(lines, fmt.Sprintf("  Status: %s", inc.Status))
			if inc.ResolvedAt != nil {
				lines = append(lines, fmt.Sprintf("  Resolved: %s", inc.ResolvedAt.Format("2006-01-02 15:04")))
			} else {
				lines = append(lines, fmt.Sprintf("  Started: %s", inc.StartedAt.Format("2006-01-02 15:04")))
			}
		}
	} else {
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("No recent incidents"))
	}

	if len(svc.Maintenances) > 0 {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render("🔧 Scheduled Maintenance:"))
		for _, maint := range svc.Maintenances {
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("• %s", maint.Title))
			lines = append(lines, fmt.Sprintf("  %s to %s", 
				maint.StartAt.Format("2006-01-02 15:04"),
				maint.EndAt.Format("2006-01-02 15:04")))
		}
	}

	if svc.ParseNote != "" {
		lines = append(lines, "")
		lines = append(lines, helpStyle.Render("Parse Note: "+svc.ParseNote))
	}

	return strings.Join(lines, "\n")
}

func (m Model) renderCommandWindow() string {
	cmdWidth := m.width - 12
	if cmdWidth < 30 {
		cmdWidth = 30
	}
	if cmdWidth > 100 {
		cmdWidth = 100
	}

	style := commandStyle.Width(cmdWidth)

	switch m.mode {
	case ModeAdd:
		content := fmt.Sprintf("➕ Add Service\nName: %s\nURL: %s\nInterval (sec): %s",
			m.nameInput.View(),
			m.urlInput.View(),
			m.intervalInput.View())
		return style.Render(content)

	case ModeEdit:
		content := fmt.Sprintf("✏️  Edit Service\nName: %s\nURL: %s\nInterval (sec): %s",
			m.nameInput.View(),
			m.urlInput.View(),
			m.intervalInput.View())
		return style.Render(content)

	case ModeConfirm:
		services := m.manager.List()
		if m.deleteTarget >= 0 && m.deleteTarget < len(services) {
			content := fmt.Sprintf("⚠️  Delete '%s'? | Enter=Yes / Esc=No", services[m.deleteTarget].Config.Name)
			return style.Render(content)
		}

	default:
		return style.Render("a: add • e: edit • d: delete • o: open • enter: refresh • r: refresh all • ?: help")
	}

	return ""
}

func (m Model) renderStatusBar(services []ServiceState) string {
	total := len(services)
	operational := 0
	degraded := 0
	disrupted := 0
	
	for _, svc := range services {
		switch svc.StatusLevel {
		case StatusOperational:
			operational++
		case StatusDegraded:
			degraded++
		case StatusMajorDisruption, StatusConnectionError, StatusParseError:
			disrupted++
		}
	}

	stats := fmt.Sprintf("📊 Total: %d • ✅ Operational: %d • ⚠️  Degraded: %d • 🚨 Issues: %d • 🕒 %s",
		total, operational, degraded, disrupted, time.Now().Format("15:04:05"))

	if m.statusMsg != "" {
		return helpStyle.Render(stats) + " • " + statusMsgStyle.Render(m.statusMsg)
	}

	return helpStyle.Render(stats)
}

func (m Model) refreshServiceCmd(index int) tea.Cmd {
	return func() tea.Msg {
		m.manager.SetInFlight(index, true)
		services := m.manager.List()
		if index >= len(services) {
			return refreshMsg{Index: index, Err: fmt.Errorf("invalid index")}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		result, err := m.fetchClient.Fetch(ctx, services[index].Config.URL)
		return refreshMsg{
			Index:       index,
			fetchResult: result,
			Err:         err,
		}
	}
}

func (m Model) refreshAllCmd() tea.Cmd {
	services := m.manager.List()
	var cmds []tea.Cmd
	for i := range services {
		if !services[i].InFlight {
			cmds = append(cmds, m.refreshServiceCmd(i))
		}
	}
	return tea.Batch(cmds...)
}

func (m Model) openURLCmd(urlStr string) tea.Cmd {
	return func() tea.Msg {
		// Use macOS 'open' command to open URL in default browser
		cmd := exec.Command("open", urlStr)
		err := cmd.Start()
		if err != nil {
			return statusMsg(fmt.Sprintf("Failed to open URL: %v", err))
		}
		return statusMsg("Opened in browser")
	}
}

type statusMsg string

func convertStatusLevel(level fetch.StatusLevel) StatusLevel {
	switch level {
	case fetch.StatusOperational:
		return StatusOperational
	case fetch.StatusPlannedMaintenance:
		return StatusPlannedMaintenance
	case fetch.StatusDegraded:
		return StatusDegraded
	case fetch.StatusMajorDisruption:
		return StatusMajorDisruption
	case fetch.StatusConnectionError:
		return StatusConnectionError
	case fetch.StatusParseError:
		return StatusParseError
	default:
		return StatusUnknown
	}
}

func convertIncidents(fetchIncidents []fetch.Incident) []Incident {
	incidents := make([]Incident, len(fetchIncidents))
	for i, inc := range fetchIncidents {
		updates := make([]IncidentUpdate, len(inc.Updates))
		for j, upd := range inc.Updates {
			updates[j] = IncidentUpdate{
				Body:      upd.Body,
				Status:    upd.Status,
				CreatedAt: upd.CreatedAt,
			}
		}
		incidents[i] = Incident{
			ID:         inc.ID,
			Title:      inc.Title,
			Status:     inc.Status,
			Impact:     inc.Impact,
			StartedAt:  inc.StartedAt,
			UpdatedAt:  inc.UpdatedAt,
			ResolvedAt: inc.ResolvedAt,
			Updates:    updates,
		}
	}
	return incidents
}

func (m *Model) updateSortedIndices() {
	services := m.manager.List()
	if len(services) == 0 {
		m.sortedIndices = []int{}
		return
	}
	
	type indexedService struct {
		service ServiceState
		origIndex int
	}
	indexed := make([]indexedService, len(services))
	for i, svc := range services {
		indexed[i] = indexedService{service: svc, origIndex: i}
	}
	sort.SliceStable(indexed, func(i, j int) bool {
		return getStatusPriority(indexed[i].service.StatusLevel) < getStatusPriority(indexed[j].service.StatusLevel)
	})
	
	m.sortedIndices = make([]int, len(indexed))
	for i, item := range indexed {
		m.sortedIndices[i] = item.origIndex
	}
}

func getStatusPriority(level StatusLevel) int {
	switch level {
	case StatusMajorDisruption:
		return 0
	case StatusConnectionError:
		return 1
	case StatusParseError:
		return 2
	case StatusDegraded:
		return 3
	case StatusPlannedMaintenance:
		return 4
	case StatusUnknown:
		return 5
	case StatusOperational:
		return 6
	default:
		return 7
	}
}

func convertMaintenances(fetchMaintenances []fetch.Maintenance) []Maintenance {
	maintenances := make([]Maintenance, len(fetchMaintenances))
	for i, maint := range fetchMaintenances {
		updates := make([]IncidentUpdate, len(maint.Updates))
		for j, upd := range maint.Updates {
			updates[j] = IncidentUpdate{
				Body:      upd.Body,
				Status:    upd.Status,
				CreatedAt: upd.CreatedAt,
			}
		}
		maintenances[i] = Maintenance{
			ID:      maint.ID,
			Title:   maint.Title,
			Status:  maint.Status,
			Impact:  maint.Impact,
			StartAt: maint.StartAt,
			EndAt:   maint.EndAt,
			Updates: updates,
		}
	}
	return maintenances
}
