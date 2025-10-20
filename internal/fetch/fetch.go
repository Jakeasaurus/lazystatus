package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
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

type IncidentUpdate struct {
	Body      string    `json:"body"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Incident struct {
	ID         string          `json:"id"`
	Title      string          `json:"name"`
	Status     string          `json:"status"`
	Impact     string          `json:"impact"`
	StartedAt  time.Time       `json:"started_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	ResolvedAt *time.Time      `json:"resolved_at,omitempty"`
	Updates    []IncidentUpdate `json:"incident_updates,omitempty"`
}

type Maintenance struct {
	ID      string          `json:"id"`
	Title   string          `json:"name"`
	Status  string          `json:"status"`
	Impact  string          `json:"impact"`
	StartAt time.Time       `json:"scheduled_for"`
	EndAt   time.Time       `json:"scheduled_until"`
	Updates []IncidentUpdate `json:"incident_updates,omitempty"`
}

type Result struct {
	Level         StatusLevel
	Label         string
	CheckedAt     time.Time
	Incidents     []Incident
	Maintenances  []Maintenance
	SourceURL     string
	ParseNote     string
}

type statuspageResponse struct {
	Page struct {
		Name      string    `json:"name"`
		UpdatedAt time.Time `json:"updated_at"`
	} `json:"page"`
	Status struct {
		Indicator   string `json:"indicator"`
		Description string `json:"description"`
	} `json:"status"`
	Incidents            []Incident    `json:"incidents"`
	ScheduledMaintenances []Maintenance `json:"scheduled_maintenances"`
}

type Client struct {
	http *http.Client
}

func NewClient() *Client {
	return &Client{
		http: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{Proxy: http.ProxyFromEnvironment},
		},
	}
}

func (c *Client) Fetch(ctx context.Context, rawURL string) (*Result, error) {
	result := &Result{
		CheckedAt: time.Now(),
		SourceURL: rawURL,
		Level:     StatusUnknown,
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		result.Level = StatusParseError
		result.ParseNote = fmt.Sprintf("Invalid URL: %v", err)
		return result, nil
	}

	var tryJSON bool
	if strings.Contains(parsedURL.Path, "/api/v2/summary.json") {
		tryJSON = true
	} else {
		apiURL := *parsedURL
		apiURL.Path = "/api/v2/summary.json"
		tryJSON = true
		rawURL = apiURL.String()
	}

	if tryJSON {
		jsonResult, err := c.fetchJSON(ctx, rawURL)
		if err == nil {
			return jsonResult, nil
		}
		result.ParseNote = fmt.Sprintf("JSON fetch failed: %v; falling back to HTML", err)
	}

	htmlResult, err := c.fetchHTML(ctx, parsedURL.String())
	if err != nil {
		result.Level = StatusConnectionError
		result.ParseNote = fmt.Sprintf("Connection error: %v", err)
		return result, nil
	}

	return htmlResult, nil
}

func (c *Client) fetchJSON(ctx context.Context, urlStr string) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "lazystatus/0.1 (+https://github.com/jakeasaurus/lazystatus)")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var spResp statuspageResponse
	if err := json.Unmarshal(body, &spResp); err != nil {
		return nil, err
	}

	result := &Result{
		CheckedAt:    time.Now(),
		SourceURL:    urlStr,
		Incidents:    spResp.Incidents,
		Maintenances: spResp.ScheduledMaintenances,
		ParseNote:    "Parsed Statuspage.io JSON API",
	}

	for i := range result.Incidents {
		if len(result.Incidents[i].Updates) > 0 {
			result.Incidents[i].StartedAt = result.Incidents[i].Updates[len(result.Incidents[i].Updates)-1].CreatedAt
			result.Incidents[i].UpdatedAt = result.Incidents[i].Updates[0].CreatedAt
		}
		if result.Incidents[i].Status == "resolved" || result.Incidents[i].Status == "completed" {
			t := result.Incidents[i].UpdatedAt
			result.Incidents[i].ResolvedAt = &t
		}
	}

	switch strings.ToLower(spResp.Status.Indicator) {
	case "none":
		result.Level = StatusOperational
		result.Label = "All Systems Operational"
	case "minor":
		result.Level = StatusDegraded
		result.Label = spResp.Status.Description
	case "major":
		result.Level = StatusMajorDisruption
		result.Label = spResp.Status.Description
	case "critical":
		result.Level = StatusMajorDisruption
		result.Label = spResp.Status.Description
	case "maintenance":
		result.Level = StatusPlannedMaintenance
		result.Label = spResp.Status.Description
	default:
		result.Level = StatusUnknown
		result.Label = spResp.Status.Description
	}

	if len(result.Maintenances) > 0 && result.Level == StatusOperational {
		result.Level = StatusPlannedMaintenance
		result.Label = "Scheduled Maintenance"
	}

	return result, nil
}

func (c *Client) fetchHTML(ctx context.Context, urlStr string) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "lazystatus/0.1 (+https://github.com/jakeasaurus/lazystatus)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	result := &Result{
		CheckedAt: time.Now(),
		SourceURL: urlStr,
		ParseNote: "Parsed HTML fallback",
		Level:     StatusUnknown,
	}

	text := extractText(doc)
	textLower := strings.ToLower(text)

	if strings.Contains(textLower, "all systems operational") ||
		(strings.Contains(textLower, "operational") && !strings.Contains(textLower, "not operational")) {
		result.Level = StatusOperational
		result.Label = "All Systems Operational"
	} else if strings.Contains(textLower, "major outage") || strings.Contains(textLower, "major disruption") {
		result.Level = StatusMajorDisruption
		result.Label = "Major Disruption Detected"
	} else if strings.Contains(textLower, "partial outage") || strings.Contains(textLower, "degraded") {
		result.Level = StatusDegraded
		result.Label = "Degraded Performance Detected"
	} else if strings.Contains(textLower, "maintenance") || strings.Contains(textLower, "scheduled") {
		result.Level = StatusPlannedMaintenance
		result.Label = "Maintenance Detected"
	} else {
		result.Level = StatusParseError
		result.Label = "Unable to determine status"
		result.ParseNote = "Could not find status keywords in HTML"
	}

	return result, nil
}

func extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}

	var text string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		text += extractText(c) + " "
	}
	return text
}
