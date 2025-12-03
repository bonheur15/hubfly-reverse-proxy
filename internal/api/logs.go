package api

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/hubfly/hubfly-reverse-proxy/internal/models"
)

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", 405)
		return
	}

	logPath := "/var/log/nginx/access.log"

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		jsonResponse(w, 200, []models.LogEntry{})
		return
	}

	file, err := os.Open(logPath)
	if err != nil {
		errorResponse(w, 500, "failed to open log file: "+err.Error())
		return
	}
	defer file.Close()

	var logs []models.LogEntry
	scanner := bufio.NewScanner(file)
	
	// Handle potentially long log lines
	const maxCapacity = 1024 * 1024
	buf := make([]byte, maxCapacity)
	scanner.Buffer(buf, maxCapacity)

	for scanner.Scan() {
		line := scanner.Text()
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

	if err := scanner.Err(); err != nil {
		errorResponse(w, 500, "error reading log file: "+err.Error())
		return
	}

	// Return all logs. Frontend can handle sorting/filtering.
	jsonResponse(w, 200, logs)
}
