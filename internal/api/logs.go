package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/hubfly/hubfly-reverse-proxy/internal/models"
)

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}

	logPath := "/var/log/hubfly/access.log"

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		jsonResponse(w, 200, []models.LogEntry{})
		return
	}

	// Parse limit query param
	limitStr := r.URL.Query().Get("limit")
	limit := 2000 // Default limit
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Use context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Use system 'tail' command to efficiently get the last N lines
	cmd := exec.CommandContext(ctx, "tail", "-n", strconv.Itoa(limit), logPath)
	output, err := cmd.Output()
	if err != nil {
		// Check if it was a timeout
		if ctx.Err() == context.DeadlineExceeded {
			errorResponse(w, 504, "timeout reading logs")
			return
		}
		errorResponse(w, 500, "failed to read logs: "+err.Error())
		return
	}

	var logs []models.LogEntry
	lines := strings.Split(string(output), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry models.LogEntry
		// Only accept valid JSON lines that match our schema
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			// Basic validation to ensure it's one of our new logs
			if entry.TimeLocal != "" {
				logs = append(logs, entry)
			}
		}
	}

	// Reverse logs to show newest first (since tail gives oldest->newest)
	// Actually, let's keep them oldest->newest (standard log order) 
	// and let the frontend reverse if needed.
	// My frontend code does: logs.value = (data || []).filter(...).reverse();
	// So frontend expects oldest->newest and reverses it to show Newest at top.
	// So we return as is.

	jsonResponse(w, 200, logs)
}
