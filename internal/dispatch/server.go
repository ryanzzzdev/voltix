package dispatch

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
)

// Server exposes POST /api/dispatch and GET /api/status behind a shared
// bearer token. It never logs the token or request bodies.
type Server struct {
	session *Session
	token   string
	mux     *http.ServeMux
	// StatusExtra, when set, merges its keys into GET /api/status.
	// Values must be JSON-marshalable; called per request.
	StatusExtra func() map[string]any
}

func NewServer(sess *Session, token string) *Server {
	s := &Server{session: sess, token: token, mux: http.NewServeMux()}
	s.mux.HandleFunc("POST /api/dispatch", s.handleDispatch)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) authorized(r *http.Request) bool {
	var tok string
	if auth := r.Header.Get("Authorization"); len(auth) > len("Bearer ") &&
		auth[:len("Bearer ")] == "Bearer " {
		tok = auth[len("Bearer "):]
	}
	return subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) == 1
}

func (s *Server) handleDispatch(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		fail(w, http.StatusUnauthorized, "missing or invalid bearer token")
		return
	}
	req, err := decodeRequest(w, r)
	if err != nil {
		fail(w, http.StatusBadRequest, "malformed or oversized body")
		return
	}
	cmd, code, msg := s.session.accept(req)
	if code != http.StatusAccepted {
		fail(w, code, msg)
		return
	}
	respond(w, http.StatusAccepted, map[string]any{
		"command_id":       cmd.CommandID,
		"revision":         cmd.Revision,
		"deadline_seconds": cmd.DeadlineMono.Seconds(),
		"accepted":         true,
		"note":             "acceptance does not imply device application",
	})
}

// handleStatus returns redacted session and command state — never the
// token. Acknowledged/measured fields are published by the control loop
// (VTX-010) via SnapshotFunc.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		fail(w, http.StatusUnauthorized, "missing or invalid bearer token")
		return
	}
	body := map[string]any{
		"controller_id":    s.session.controllerID,
		"boot_session_id":  s.session.bootID,
		"dispatch_active":  false,
		"requested_target": nil,
	}
	if cmd, ok := s.session.Active(); ok {
		body["dispatch_active"] = true
		body["requested_target"] = cmd.TargetKW
		body["revision"] = cmd.Revision
		body["command_id"] = cmd.CommandID
	}
	if s.StatusExtra != nil {
		for k, v := range s.StatusExtra() {
			body[k] = v
		}
	}
	respond(w, http.StatusOK, body)
}

func respond(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	respond(w, code, map[string]any{"error": msg})
}
