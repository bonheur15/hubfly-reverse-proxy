package logmanager

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetAccessLogs(t *testing.T) {
	// Setup temp dir
	tmpDir, err := os.MkdirTemp("", "logtest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create sample log file
	siteID := "example.com"
	logContent := `127.0.0.1 - - [26/Dec/2025:10:00:00 +0000] "GET /old HTTP/1.1" 200 123 "-" "Agent" "0.001"
127.0.0.1 - - [26/Dec/2025:10:05:00 +0000] "GET /search HTTP/1.1" 404 456 "-" "Agent" "0.002"
127.0.0.1 - - [26/Dec/2025:10:10:00 +0000] "POST /api HTTP/1.1" 201 789 "-" "Agent" "0.003"
`
	if err := os.WriteFile(filepath.Join(tmpDir, siteID+".access.log"), []byte(logContent), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(tmpDir)

	// Test 1: List all
	logs, err := mgr.GetAccessLogs(siteID, LogOptions{Limit: 10})
	if err != nil {
		t.Fatalf("GetAccessLogs failed: %v", err)
	}
	if len(logs) != 3 {
		t.Errorf("Expected 3 logs, got %d", len(logs))
	}
	if logs[0].Request != "GET /old HTTP/1.1" {
		t.Errorf("First log request mismatch: %s", logs[0].Request)
	}

	// Test 2: Search
	logs, err = mgr.GetAccessLogs(siteID, LogOptions{Limit: 10, Search: "POST"})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Errorf("Expected 1 log, got %d", len(logs))
	}
	if logs[0].Request != "POST /api HTTP/1.1" {
		t.Errorf("Search mismatch")
	}

	// Test 3: Time Filter
	since, _ := time.Parse(nginxTimeLayout, "26/Dec/2025:10:04:00 +0000")
	logs, err = mgr.GetAccessLogs(siteID, LogOptions{Limit: 10, Since: since})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Errorf("Expected 2 logs since 10:04, got %d", len(logs))
	}

	// Test 4: Limit
	logs, err = mgr.GetAccessLogs(siteID, LogOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 {
		t.Errorf("Expected 1 log, got %d", len(logs))
	}
	// Should be the LAST one (latest)
	if logs[0].Request != "POST /api HTTP/1.1" {
		t.Errorf("Limit should return last log, got %s", logs[0].Request)
	}
}

func TestGetErrorLogs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logtest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	siteID := "example.com"
	logContent := `2025/12/26 10:00:00 [error] 123#123: *1 open() failed (2: No such file)
2025/12/26 10:10:00 [warn] 123#123: *2 something weird
`
	if err := os.WriteFile(filepath.Join(tmpDir, siteID+".error.log"), []byte(logContent), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(tmpDir)

	logs, err := mgr.GetErrorLogs(siteID, LogOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 {
		t.Errorf("Expected 2 logs, got %d", len(logs))
	}
	if logs[0].Level != "error" {
		t.Errorf("Expected error level, got %s", logs[0].Level)
	}
	if logs[1].Level != "warn" {
		t.Errorf("Expected warn level, got %s", logs[1].Level)
	}
}
