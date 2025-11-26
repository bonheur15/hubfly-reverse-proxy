package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/hubfly/hubfly-reverse-proxy/internal/certbot"
	"github.com/hubfly/hubfly-reverse-proxy/internal/models"
	"github.com/hubfly/hubfly-reverse-proxy/internal/nginx"
	"github.com/hubfly/hubfly-reverse-proxy/internal/store"
)

type Server struct {
	Store   store.Store
	Nginx   *nginx.Manager
	Certbot *certbot.Manager
}

func NewServer(s store.Store, n *nginx.Manager, c *certbot.Manager) *Server {
	return &Server{
		Store:   s,
		Nginx:   n,
		Certbot: c,
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/sites", s.handleSites)       // GET, POST
	mux.HandleFunc("/v1/sites/", s.handleSiteDetail) // GET, DELETE, PATCH
	mux.HandleFunc("/v1/streams", s.handleStreams)       // GET, POST
	mux.HandleFunc("/v1/streams/", s.handleStreamDetail) // GET, DELETE
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, 200, map[string]string{"status": "ok"})
}

// ... handleSites and handleSiteDetail omitted for brevity ...

func (s *Server) handleStreams(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		streams, err := s.Store.ListStreams()
		if err != nil {
			errorResponse(w, 500, err.Error())
			return
		}
		jsonResponse(w, 200, streams)
	case http.MethodPost:
		var stream models.Stream
		if err := json.NewDecoder(r.Body).Decode(&stream); err != nil {
			errorResponse(w, 400, "invalid json")
			return
		}
		if stream.ID == "" {
			// Use listen port as ID if not provided, or unique ID
			// Simple: "stream-PORT"
			// But user might provide ID.
			if stream.ListenPort == 0 {
				errorResponse(w, 400, "listen_port is required")
				return
			}
		}
		if stream.ID == "" {
			// Generate ID
			stream.ID = fmt.Sprintf("stream-%d", stream.ListenPort)
		}
		if stream.Protocol == "" {
			stream.Protocol = "tcp"
		}

		stream.CreatedAt = time.Now()
		stream.UpdatedAt = time.Now()
		stream.Status = "provisioning"

		if err := s.Store.SaveStream(&stream); err != nil {
			errorResponse(w, 500, err.Error())
			return
		}

		streamCopy := stream
		go s.provisionStream(&streamCopy)

		jsonResponse(w, 201, stream)
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) handleStreamDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/v1/streams/"):]
	if id == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		stream, err := s.Store.GetStream(id)
		if err != nil {
			errorResponse(w, 404, "stream not found")
			return
		}
		jsonResponse(w, 200, stream)
	case http.MethodDelete:
		if err := s.Nginx.DeleteStream(id); err != nil {
			errorResponse(w, 500, "failed to remove nginx config: "+err.Error())
			return
		}

		if err := s.Store.DeleteStream(id); err != nil {
			errorResponse(w, 500, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) provisionStream(stream *models.Stream) {
	staging, err := s.Nginx.GenerateStreamConfig(stream)
	if err != nil {
		s.updateStreamStatus(stream.ID, "error", "config gen failed: "+err.Error())
		return
	}

	// Validate? (skip for now as per Manager)

	if err := s.Nginx.ApplyStream(stream.ID, staging); err != nil {
		s.updateStreamStatus(stream.ID, "error", "apply failed: "+err.Error())
		return
	}

	s.updateStreamStatus(stream.ID, "active", "")
}

func (s *Server) updateStreamStatus(id, status, msg string) {
	stream, err := s.Store.GetStream(id)
	if err != nil {
		return
	}
	stream.Status = status
	stream.ErrorMessage = msg
	stream.UpdatedAt = time.Now()
	s.Store.SaveStream(stream)
}

func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sites, err := s.Store.ListSites()
		if err != nil {
			errorResponse(w, 500, err.Error())
			return
		}
		jsonResponse(w, 200, sites)
	case http.MethodPost:
		var site models.Site
		if err := json.NewDecoder(r.Body).Decode(&site); err != nil {
			errorResponse(w, 400, "invalid json")
			return
		}
		if site.ID == "" {
			site.ID = site.Domain // Simple ID generation
		}
		site.CreatedAt = time.Now()
		site.UpdatedAt = time.Now()
		site.Status = "provisioning"

		// save initial state
		if err := s.Store.SaveSite(&site); err != nil {
			errorResponse(w, 500, err.Error())
			return
		}

		// Apply Nginx Config (async)
		// We pass a copy to avoid race with jsonResponse which reads 'site'
		siteCopy := site
		go s.provisionSite(&siteCopy)

		jsonResponse(w, 201, site)
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) handleSiteDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/v1/sites/"):]
	if id == "" {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		site, err := s.Store.GetSite(id)
		if err != nil {
			errorResponse(w, 404, "site not found")
			return
		}
		jsonResponse(w, 200, site)
	case http.MethodDelete:
		// Check if revoke requested
		revoke := r.URL.Query().Get("revoke_cert") == "true"

		site, err := s.Store.GetSite(id)
		if err != nil {
			errorResponse(w, 404, "site not found")
			return
		}

		if revoke && site.SSL {
			if err := s.Certbot.Revoke(site.Domain); err != nil {
				log.Printf("failed to revoke cert for %s: %v", site.Domain, err)
				// continue to delete
			}
		}

		if err := s.Nginx.Delete(id); err != nil {
			errorResponse(w, 500, "failed to remove nginx config: "+err.Error())
			return
		}

		if err := s.Store.DeleteSite(id); err != nil {
			errorResponse(w, 500, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]string{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) provisionSite(site *models.Site) {
	// 1. Generate Nginx Config (HTTP)
	// 2. Test & Reload
	// 3. If SSL, Issue Cert -> Regenerate (SSL) -> Reload

	// Initial render (might be HTTP only first if SSL requested but not present)
	// For MVP simplicity, we trust the 'SSL' flag.
	// In real life, we first render HTTP-only to pass challenge, then SSL.

	// Logic:
	// If SSL is requested, we force SSL=false for first pass to ensure Nginx starts and serves challenge.
	// Then we run certbot.
	// Then we set SSL=true and re-render.

	originalSSL := site.SSL
	if originalSSL {
		site.SSL = false // Temporary disable for challenge
	}

	staging, err := s.Nginx.GenerateConfig(site)
	if err != nil {
		s.updateStatus(site.ID, "error", "config gen failed: "+err.Error())
		return
	}

	if err := s.Nginx.Validate(staging); err != nil {
		s.updateStatus(site.ID, "error", "config invalid: "+err.Error())
		return
	}

	if err := s.Nginx.Apply(site.ID, staging); err != nil {
		s.updateStatus(site.ID, "error", "apply failed: "+err.Error())
		return
	}

	if !originalSSL {
		s.updateStatus(site.ID, "active", "")
		return
	}

	// Handle SSL
	s.updateStatus(site.ID, "provisioning", "issuing certificate")
	if err := s.Certbot.Issue(site.Domain); err != nil {
		s.updateStatus(site.ID, "cert-failed", err.Error())
		return
	}

	// Re-apply with SSL
	site.SSL = true
	site.CertIssueStatus = "valid"
	// Update store with SSL=true
	s.Store.SaveSite(site)

	stagingSSL, err := s.Nginx.GenerateConfig(site)
	if err != nil {
		s.updateStatus(site.ID, "error", "ssl config gen failed: "+err.Error())
		return
	}

	// Validate & Apply
	if err := s.Nginx.Apply(site.ID, stagingSSL); err != nil {
		s.updateStatus(site.ID, "error", "ssl apply failed: "+err.Error())
		return
	}

	s.updateStatus(site.ID, "active", "")
}

func (s *Server) updateStatus(id, status, msg string) {
	site, err := s.Store.GetSite(id)
	if err != nil {
		return
	}
	site.Status = status
	site.ErrorMessage = msg
	site.UpdatedAt = time.Now()
	s.Store.SaveSite(site)
}

func jsonResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

func errorResponse(w http.ResponseWriter, code int, msg string) {
	jsonResponse(w, code, map[string]interface{}{
		"error": msg,
		"code":  code,
	})
}
