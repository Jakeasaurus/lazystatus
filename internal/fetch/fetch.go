package fetch

import (
	"context"
	"encoding/json"
	"encoding/xml"
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

type rssItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Link        string `xml:"link"`
	PubDate     string `xml:"pubDate"`
}

type rssFeed struct {
	Channel struct {
		Title string    `xml:"title"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type atomEntry struct {
	Title   string `xml:"title"`
	Summary string `xml:"summary"`
	Updated string `xml:"updated"`
	Link    struct {
		Href string `xml:"href,attr"`
	} `xml:"link"`
}

type atomFeed struct {
	Title   string       `xml:"title"`
	Entries []atomEntry `xml:"entry"`
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

	// Check if URL is an RSS/Atom feed
	if strings.HasSuffix(parsedURL.Path, ".rss") || strings.HasSuffix(parsedURL.Path, ".xml") || 
	   strings.Contains(parsedURL.Path, "/rss") || strings.Contains(parsedURL.Path, "/feed") ||
	   strings.Contains(parsedURL.Path, "/atom") {
		rssResult, err := c.fetchRSS(ctx, rawURL)
		if err == nil {
			return rssResult, nil
		}
		result.ParseNote = fmt.Sprintf("RSS fetch failed: %v; trying JSON", err)
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

func (c *Client) fetchRSS(ctx context.Context, urlStr string) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "lazystatus/0.1 (+https://github.com/jakeasaurus/lazystatus)")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml")

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

	result := &Result{
		CheckedAt: time.Now(),
		SourceURL: urlStr,
		ParseNote: "Parsed RSS/Atom feed",
	}

	// Try RSS first
	var rss rssFeed
	if err := xml.Unmarshal(body, &rss); err == nil && len(rss.Channel.Items) > 0 {
		return parseRSSItems(rss.Channel.Items, result), nil
	}

	// Try Atom
	var atom atomFeed
	if err := xml.Unmarshal(body, &atom); err == nil && len(atom.Entries) > 0 {
		return parseAtomEntries(atom.Entries, result), nil
	}

	return nil, fmt.Errorf("failed to parse as RSS or Atom feed")
}

func parseRSSItems(items []rssItem, result *Result) *Result {
	if len(items) == 0 {
		result.Level = StatusUnknown
		result.Label = "No feed items found"
		return result
	}

	// Status is determined by the MOST RECENT item only
	latestItem := items[0]
	latestLower := strings.ToLower(latestItem.Title + " " + latestItem.Description)
	var currentStatus StatusLevel = StatusOperational
	var currentLabel string = "All Systems Operational"
	
	// Check if latest item indicates everything is OK
	if strings.Contains(latestLower, "resolved") || strings.Contains(latestLower, "operating normally") {
		currentStatus = StatusOperational
		currentLabel = "All Systems Operational"
	} else if strings.Contains(latestLower, "major") || strings.Contains(latestLower, "outage") || strings.Contains(latestLower, "disruption") {
		currentStatus = StatusMajorDisruption
		currentLabel = latestItem.Title
	} else if strings.Contains(latestLower, "degraded") || strings.Contains(latestLower, "degradation") ||
		strings.Contains(latestLower, "impact") || strings.Contains(latestLower, "latenc") ||
		strings.Contains(latestLower, "error") {
		currentStatus = StatusDegraded
		currentLabel = latestItem.Title
	} else if strings.Contains(latestLower, "maintenance") || strings.Contains(latestLower, "scheduled") {
		currentStatus = StatusPlannedMaintenance
		currentLabel = latestItem.Title
	}

	// Collect all incidents from last 7 days for history
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	var recentIncidents []Incident

	for _, item := range items {
		// Parse pubDate (RSS format: "Mon, 20 Oct 2025 15:53:00 PDT")
		pubDate, err := time.Parse("Mon, 02 Jan 2006 15:04:05 MST", item.PubDate)
		if err != nil {
			// If parse fails, include it anyway (assume recent)
			pubDate = time.Now()
		}

		// Skip items older than 7 days
		if pubDate.Before(sevenDaysAgo) {
			continue
		}

		titleLower := strings.ToLower(item.Title)
		descLower := strings.ToLower(item.Description)
		combined := titleLower + " " + descLower

		// Determine if this is an incident or maintenance
		if strings.Contains(combined, "maintenance") || strings.Contains(combined, "scheduled") {
			// Skip maintenance items for incident list
			continue
		}

		// Check if it's resolved - do this FIRST
		var resolvedAt *time.Time
		itemStatus := "investigating"
		isResolved := strings.Contains(combined, "resolved") || strings.Contains(combined, "operating normally")
		if isResolved {
			resolvedAt = &pubDate
			itemStatus = "resolved"
		}

		// Determine impact level for incident history
		impact := "minor"
		if strings.Contains(combined, "major") || strings.Contains(combined, "outage") || strings.Contains(combined, "disruption") {
			impact = "major"
		} else if strings.Contains(combined, "degraded") || strings.Contains(combined, "degradation") ||
			strings.Contains(combined, "impact") || strings.Contains(combined, "latenc") ||
			strings.Contains(combined, "error") {
			impact = "minor"
		} else if !isResolved {
			// Unrecognized non-resolved item, skip it
			continue
		}

		// Add to incidents list
		recentIncidents = append(recentIncidents, Incident{
			ID:         item.Link,
			Title:      item.Title,
			Status:     itemStatus,
			Impact:     impact,
			StartedAt:  pubDate,
			UpdatedAt:  pubDate,
			ResolvedAt: resolvedAt,
		})
	}

	result.Level = currentStatus
	result.Label = currentLabel
	result.Incidents = recentIncidents

	// Add maintenance info if latest item is about maintenance
	if currentStatus == StatusPlannedMaintenance {
		pubDate, _ := time.Parse("Mon, 02 Jan 2006 15:04:05 MST", latestItem.PubDate)
		if pubDate.IsZero() {
			pubDate = time.Now()
		}
		result.Maintenances = []Maintenance{{
			ID:      latestItem.Link,
			Title:   latestItem.Title,
			Status:  "scheduled",
			StartAt: pubDate,
			EndAt:   pubDate.Add(24 * time.Hour),
		}}
	}

	return result
}

func parseAtomEntries(entries []atomEntry, result *Result) *Result {
	if len(entries) == 0 {
		result.Level = StatusUnknown
		result.Label = "No feed entries found"
		return result
	}

	// Convert first entry to RSS format and reuse logic
	item := rssItem{
		Title:       entries[0].Title,
		Description: entries[0].Summary,
		Link:        entries[0].Link.Href,
		PubDate:     entries[0].Updated,
	}
	return parseRSSItems([]rssItem{item}, result)
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
