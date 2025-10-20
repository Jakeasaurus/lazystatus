package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type StatusLevel int

const (
	StatusUnknown StatusLevel = iota
	StatusOperational
	StatusPlannedMaintenance
	StatusDegraded
	StatusMajorDisruption
	StatusConnectionError
	StatusParseError
)

func (s StatusLevel) String() string {
	switch s {
	case StatusOperational:
		return "Operational"
	case StatusPlannedMaintenance:
		return "Planned Maintenance"
	case StatusDegraded:
		return "Degraded Performance"
	case StatusMajorDisruption:
		return "Major Disruption"
	case StatusConnectionError:
		return "Connection Error"
	case StatusParseError:
		return "Parse Error"
	default:
		return "Unknown"
	}
}

func (s StatusLevel) Color() string {
	switch s {
	case StatusOperational:
		return "#04B575"
	case StatusPlannedMaintenance:
		return "#5FAFFF"
	case StatusDegraded:
		return "#FFAF5F"
	case StatusMajorDisruption, StatusConnectionError, StatusParseError:
		return "#FF5F87"
	default:
		return "#626262"
	}
}

type IncidentUpdate struct {
	Body      string    `json:"body"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Incident struct {
	ID         string            `json:"id"`
	Title      string            `json:"title"`
	Status     string            `json:"status"`
	Impact     string            `json:"impact"`
	StartedAt  time.Time         `json:"started_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
	ResolvedAt *time.Time        `json:"resolved_at,omitempty"`
	Updates    []IncidentUpdate  `json:"updates,omitempty"`
}

type Maintenance struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Status    string            `json:"status"`
	Impact    string            `json:"impact"`
	StartAt   time.Time         `json:"start_at"`
	EndAt     time.Time         `json:"end_at"`
	Updates   []IncidentUpdate  `json:"updates,omitempty"`
}

type ServiceConfig struct {
	Name                    string    `json:"name"`
	URL                     string    `json:"url"`
	RefreshIntervalSeconds  int       `json:"refresh_interval"`
	LastChecked             time.Time `json:"last_checked,omitempty"`
	CurrentStatus           string    `json:"current_status,omitempty"`
}

type ServiceState struct {
	Config        ServiceConfig
	NextRefreshAt time.Time
	InFlight      bool
	StatusLevel   StatusLevel
	Incidents     []Incident
	Maintenances  []Maintenance
	ParseNote     string
	LastError     string
}

type Settings struct {
	DefaultRefreshInterval int `json:"default_refresh_interval"`
}

type Config struct {
	Services []ServiceConfig `json:"services"`
	Settings Settings        `json:"settings"`
}

type ServiceManager struct {
	mu       sync.RWMutex
	config   Config
	states   []ServiceState
	filePath string
}

func NewServiceManager() (*ServiceManager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".lazystatus")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	filePath := filepath.Join(configDir, "config.json")

	sm := &ServiceManager{
		filePath: filePath,
		config: Config{
			Settings: Settings{
				DefaultRefreshInterval: 30,
			},
		},
	}

	if err := sm.Load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	sm.initStates()
	return sm, nil
}

func (sm *ServiceManager) Load() error {
	data, err := os.ReadFile(sm.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &sm.config)
}

func (sm *ServiceManager) Save() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	data, err := json.MarshalIndent(sm.config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(sm.filePath, data, 0644)
}

func (sm *ServiceManager) initStates() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.states = make([]ServiceState, len(sm.config.Services))
	for i, cfg := range sm.config.Services {
		sm.states[i] = ServiceState{
			Config:        cfg,
			NextRefreshAt: time.Now(),
			StatusLevel:   StatusUnknown,
		}
	}
}

func (sm *ServiceManager) List() []ServiceState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make([]ServiceState, len(sm.states))
	copy(result, sm.states)
	return result
}

func (sm *ServiceManager) Add(cfg ServiceConfig) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if cfg.RefreshIntervalSeconds == 0 {
		cfg.RefreshIntervalSeconds = sm.config.Settings.DefaultRefreshInterval
	}

	sm.config.Services = append(sm.config.Services, cfg)
	sm.states = append(sm.states, ServiceState{
		Config:        cfg,
		NextRefreshAt: time.Now(),
		StatusLevel:   StatusUnknown,
	})

	return nil
}

func (sm *ServiceManager) Update(index int, cfg ServiceConfig) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if index < 0 || index >= len(sm.config.Services) {
		return fmt.Errorf("index out of range")
	}

	if cfg.RefreshIntervalSeconds == 0 {
		cfg.RefreshIntervalSeconds = sm.config.Settings.DefaultRefreshInterval
	}

	sm.config.Services[index] = cfg
	sm.states[index].Config = cfg

	return nil
}

func (sm *ServiceManager) Remove(index int) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if index < 0 || index >= len(sm.config.Services) {
		return fmt.Errorf("index out of range")
	}

	sm.config.Services = append(sm.config.Services[:index], sm.config.Services[index+1:]...)
	sm.states = append(sm.states[:index], sm.states[index+1:]...)

	return nil
}

func (sm *ServiceManager) SetInFlight(index int, inFlight bool) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if index < 0 || index >= len(sm.states) {
		return fmt.Errorf("index out of range")
	}

	sm.states[index].InFlight = inFlight
	return nil
}

func (sm *ServiceManager) UpdateStatus(index int, level StatusLevel, incidents []Incident, maintenances []Maintenance, parseNote, lastError string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if index < 0 || index >= len(sm.states) {
		return fmt.Errorf("index out of range")
	}

	now := time.Now()
	sm.states[index].StatusLevel = level
	sm.states[index].Incidents = incidents
	sm.states[index].Maintenances = maintenances
	sm.states[index].ParseNote = parseNote
	sm.states[index].LastError = lastError
	sm.states[index].InFlight = false
	sm.states[index].Config.LastChecked = now
	sm.states[index].Config.CurrentStatus = level.String()
	
	interval := time.Duration(sm.states[index].Config.RefreshIntervalSeconds) * time.Second
	sm.states[index].NextRefreshAt = now.Add(interval)

	sm.config.Services[index].LastChecked = now
	sm.config.Services[index].CurrentStatus = level.String()

	return nil
}

func (sm *ServiceManager) GetNextRefreshAt(index int) (time.Time, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if index < 0 || index >= len(sm.states) {
		return time.Time{}, fmt.Errorf("index out of range")
	}

	return sm.states[index].NextRefreshAt, nil
}

func (sm *ServiceManager) GetDefaultInterval() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.config.Settings.DefaultRefreshInterval
}
