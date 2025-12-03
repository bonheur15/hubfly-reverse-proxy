package api

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
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
	mux.HandleFunc("/v1/logs", s.handleLogs)             // GET
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
		if stream.ListenPort == 0 {
			streams, err := s.Store.ListStreams()
			if err != nil {
				errorResponse(w, 500, "failed to list streams: "+err.Error())
				return
			}

			usedPorts := make(map[int]bool)
			for _, str := range streams {
				usedPorts[str.ListenPort] = true
			}

			var candidates []int
			for p := 30000; p <= 30100; p++ {
				if !usedPorts[p] {
					candidates = append(candidates, p)
				}
			}

			if len(candidates) == 0 {
				errorResponse(w, 500, "no available ports in range 30000-30100")
				return
			}

			stream.ListenPort = candidates[rand.Intn(len(candidates))]
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
		// Get stream to know the port
		stream, err := s.Store.GetStream(id)
		if err != nil {
			errorResponse(w, 404, "stream not found")
			return
		}
		port := stream.ListenPort

		if err := s.Store.DeleteStream(id); err != nil {
			errorResponse(w, 500, err.Error())
			return
		}

		// Reconcile Nginx Config for this port
		go s.reconcileStreams(port)

		jsonResponse(w, 200, map[string]string{"status": "deleted"})
	default:
		http.Error(w, "method not allowed", 405)
	}
}

func (s *Server) reconcileStreams(port int) {
	// 1. List all streams
	allStreams, err := s.Store.ListStreams()
	if err != nil {
		log.Printf("reconcile error: failed to list streams: %v", err)
		return
	}

	// 2. Filter by port
	var portStreams []models.Stream
	for _, str := range allStreams {
		if str.ListenPort == port {
			portStreams = append(portStreams, str)
		}
	}

	// 3. Rebuild Config
	if err := s.Nginx.RebuildStreamConfig(port, portStreams); err != nil {
		log.Printf("reconcile error: failed to rebuild config for port %d: %v", port, err)
		// Update status for all affected streams?
		// For MVP, we log. In production, we should update status of all portStreams to 'error'.
		return
	}

	// Success: Update status of these streams to active
	for _, str := range portStreams {
		if str.Status != "active" {
			s.updateStreamStatus(str.ID, "active", "")
		}
	}
}

func (s *Server) provisionStream(stream *models.Stream) {
	// Deprecated: use reconcileStreams
	s.reconcileStreams(stream.ListenPort)
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
