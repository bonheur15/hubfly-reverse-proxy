package logmanager

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type LogEntry struct {
	Raw           string    `json:"raw"`
	RemoteAddr    string    `json:"remote_addr,omitempty"`
	RemoteUser    string    `json:"remote_user,omitempty"`
	TimeLocal     time.Time `json:"time_local,omitempty"`
	Request       string    `json:"request,omitempty"`
	Status        int       `json:"status,omitempty"`
	BodyBytesSent int64     `json:"body_bytes_sent,omitempty"`
	Referer       string    `json:"referer,omitempty"`
	UserAgent     string    `json:"user_agent,omitempty"`
	RequestTime   float64   `json:"request_time,omitempty"`
}

type ErrorLogEntry struct {
	Raw       string    `json:"raw"`
	TimeLocal time.Time `json:"time_local,omitempty"`
	Level     string    `json:"level,omitempty"`
	Message   string    `json:"message,omitempty"`
}

type LogOptions struct {
	Limit  int       `json:"limit"`
	Since  time.Time `json:"since"`
	Until  time.Time `json:"until"`
	Search string    `json:"search"`
}

type Manager struct {
	LogDir string
}

func NewManager(logDir string) *Manager {
	return &Manager{LogDir: logDir}
}

// Access Log Regex
// $remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent" "$request_time"
// Example: 127.0.0.1 - - [26/Dec/2025:10:00:00 +0000] "GET / HTTP/1.1" 200 612 "-" "Mozilla/5.0" "0.001"
var accessLogRegex = regexp.MustCompile(`^(\S+) - (\S+) \[([^\]]+)\] "([^"]+)" (\d+) (\d+) "([^"]*)" "([^"]*)" "([^"]*)"$`)

const nginxTimeLayout = "02/Jan/2006:15:04:05 -0700"
const errorLogTimeLayout = "2006/01/02 15:04:05"

func (m *Manager) GetAccessLogs(siteID string, opts LogOptions) ([]LogEntry, error) {
	filename := filepath.Join(m.LogDir, siteID+".access.log")
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return []LogEntry{}, nil // Return empty if file doesn't exist yet
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []LogEntry
	scanner := bufio.NewScanner(file)
	
	// Buffer for lines to process reverse or forward? 
	// To support "since", we should scan forward. 
	// To support "limit" (last N), we usually want the end.
	// Combining: Scan all, filter, then take last N. 
	// Optimization: If no search/since, seek to end? (Skip for now for simplicity)

	for scanner.Scan() {
		line := scanner.Text()
		
		// 1. Basic Search Filter
		if opts.Search != "" && !strings.Contains(line, opts.Search) {
			continue
		}

		// 2. Parse
		matches := accessLogRegex.FindStringSubmatch(line)
		if len(matches) != 10 {
			// Failed to parse, maybe just return raw?
			// For now, skip or include raw.
			if matches == nil && opts.Search == "" {
                 // Try to include it if it matches search or no search
                 // But we can't filter by time if we can't parse.
                 // Let's skip malformed lines if we have time filter.
			}
			continue
		}

		t, err := time.Parse(nginxTimeLayout, matches[3])
		if err != nil {
			continue
		}

		// 3. Time Filter
		if !opts.Since.IsZero() && t.Before(opts.Since) {
			continue
		}
		if !opts.Until.IsZero() && t.After(opts.Until) {
			continue
		}

		status, _ := strconv.Atoi(matches[5])
		bytesSent, _ := strconv.ParseInt(matches[6], 10, 64)
		reqTime, _ := strconv.ParseFloat(matches[9], 64)

		entry := LogEntry{
			Raw:           line,
			RemoteAddr:    matches[1],
			RemoteUser:    matches[2],
			TimeLocal:     t,
			Request:       matches[4],
			Status:        status,
			BodyBytesSent: bytesSent,
			Referer:       matches[7],
			UserAgent:     matches[8],
			RequestTime:   reqTime,
		}

		entries = append(entries, entry)
	}

	// Apply Limit (Take last N)
	if opts.Limit > 0 && len(entries) > opts.Limit {
		entries = entries[len(entries)-opts.Limit:]
	}

	return entries, nil
}

func (m *Manager) GetErrorLogs(siteID string, opts LogOptions) ([]ErrorLogEntry, error) {
	filename := filepath.Join(m.LogDir, siteID+".error.log")
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return []ErrorLogEntry{}, nil
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []ErrorLogEntry
	scanner := bufio.NewScanner(file)

	// Regex for error log
	// 2025/12/26 10:00:00 [error] ...
	// Simple split by space usually works for first parts
	
	for scanner.Scan() {
		line := scanner.Text()

		if opts.Search != "" && !strings.Contains(line, opts.Search) {
			continue
		}

		// Parse timestamp
		// Format: YYYY/MM/DD HH:MM:SS
		if len(line) < 19 {
			continue
		}
		timeStr := line[:19]
		t, err := time.Parse(errorLogTimeLayout, timeStr)
		if err != nil {
			// Check if it's a continuation line or different format?
			// Just include raw if we can't parse time but search matched?
			// If filtering by time, we must skip.
			if !opts.Since.IsZero() || !opts.Until.IsZero() {
				continue
			}
		} else {
			if !opts.Since.IsZero() && t.Before(opts.Since) {
				continue
			}
			if !opts.Until.IsZero() && t.After(opts.Until) {
				continue
			}
		}

		// Extract Level
		// Look for [level]
		level := "unknown"
		startBracket := strings.Index(line, "[")
		endBracket := strings.Index(line, "]")
		if startBracket != -1 && endBracket != -1 && endBracket > startBracket {
			level = line[startBracket+1 : endBracket]
		}
		
		msg := ""
		if endBracket != -1 && len(line) > endBracket+1 {
			msg = strings.TrimSpace(line[endBracket+1:])
		} else {
			msg = line
		}

		entries = append(entries, ErrorLogEntry{
			Raw:       line,
			TimeLocal: t,
			Level:     level,
			Message:   msg,
		})
	}

	if opts.Limit > 0 && len(entries) > opts.Limit {
		entries = entries[len(entries)-opts.Limit:]
	}

	return entries, nil
}
