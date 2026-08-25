package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"scenicpermit/internal/application"
)

type Server struct {
	service *application.Service
	logger  *slog.Logger
	mux     *http.ServeMux
}

func New(service *application.Service, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{service: service, logger: logger, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler { return s.recoverer(s.securityHeaders(s.accessLog(s.mux))) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.HealthHandler)
	s.mux.HandleFunc("GET /app", s.AppHandler)
	s.mux.HandleFunc("GET /app/", s.AppHandler)
	s.mux.HandleFunc("GET /assets/app.css", s.CSSHandler)
	s.mux.HandleFunc("GET /assets/app.js", s.JSHandler)
	s.mux.HandleFunc("GET /api/v1/batches", s.ListBatchesHandler)
	s.mux.HandleFunc("POST /api/v1/batches", s.CreateBatchHandler)
	s.mux.HandleFunc("PATCH /api/v1/batches/{batchID}", s.UpdateBatchHandler)
	s.mux.HandleFunc("GET /api/v1/batches/{batchID}", s.BatchDetailHandler)
	s.mux.HandleFunc("POST /api/v1/batches/{batchID}/units", s.AddUnitHandler)
	s.mux.HandleFunc("PUT /api/v1/batches/{batchID}/units/{unitID}", s.UpdateUnitHandler)
	s.mux.HandleFunc("DELETE /api/v1/batches/{batchID}/units/{unitID}", s.RemoveUnitHandler)
	s.mux.HandleFunc("POST /api/v1/batches/{batchID}/plan/preflight", s.PreflightPlanHandler)
	s.mux.HandleFunc("POST /api/v1/batches/{batchID}/submit", s.SubmitPlanHandler)
	s.mux.HandleFunc("POST /api/v1/batches/{batchID}/results", s.RecordResultHandler)
	s.mux.HandleFunc("POST /api/v1/batches/{batchID}/results/batch", s.RecordResultsHandler)
	s.mux.HandleFunc("POST /api/v1/batches/{batchID}/remediations", s.OpenRemediationHandler)
	s.mux.HandleFunc("POST /api/v1/batches/{batchID}/remediations/{remediationID}/complete", s.CompleteRemediationHandler)
	s.mux.HandleFunc("PATCH /api/v1/batches/{batchID}/remediations/{remediationID}", s.ChangeRemediationHandler)
	s.mux.HandleFunc("POST /api/v1/batches/{batchID}/approve", s.ApproveHandler)
	s.mux.HandleFunc("GET /api/v1/permits/verify", s.VerifyPermitsHandler)
	s.mux.HandleFunc("GET /api/v1/permits/lookup", s.VerifyPermitHandler)
	s.mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app", http.StatusTemporaryRedirect)
	})
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		if !strings.HasPrefix(r.URL.Path, "/assets/") {
			s.logger.Info("HTTP 请求", "method", r.Method, "path", r.URL.Path, "status", sw.status, "duration", time.Since(started))
		}
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("HTTP 处理发生异常", "error", recovered)
				writeError(w, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
