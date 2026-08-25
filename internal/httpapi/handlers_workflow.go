package httpapi

import (
	"net/http"
	"scenicpermit/internal/application"
	"scenicpermit/internal/domain"
	"strconv"
)

func (s *Server) RecordResultHandler(w http.ResponseWriter, r *http.Request) {
	var command application.RecordResultCommand
	if err := decodeJSON(w, r, &command); err != nil {
		handleError(w, err)
		return
	}
	command.BatchID = r.PathValue("batchID")
	result, err := s.service.RecordResult(r.Context(), command)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) RecordResultsHandler(w http.ResponseWriter, r *http.Request) {
	var command application.RecordResultsCommand
	if err := decodeJSON(w, r, &command); err != nil {
		handleError(w, err)
		return
	}
	command.BatchID = r.PathValue("batchID")
	result, err := s.service.RecordResults(r.Context(), command)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) OpenRemediationHandler(w http.ResponseWriter, r *http.Request) {
	var command application.OpenRemediationCommand
	if err := decodeJSON(w, r, &command); err != nil {
		handleError(w, err)
		return
	}
	command.BatchID = r.PathValue("batchID")
	result, err := s.service.OpenRemediation(r.Context(), command)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) CompleteRemediationHandler(w http.ResponseWriter, r *http.Request) {
	var command application.CompleteRemediationCommand
	if err := decodeJSON(w, r, &command); err != nil {
		handleError(w, err)
		return
	}
	command.BatchID, command.RemediationID = r.PathValue("batchID"), r.PathValue("remediationID")
	result, err := s.service.CompleteRemediation(r.Context(), command)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) ChangeRemediationHandler(w http.ResponseWriter, r *http.Request) {
	var command application.ChangeRemediationCommand
	if err := decodeJSON(w, r, &command); err != nil {
		handleError(w, err)
		return
	}
	command.BatchID, command.RemediationID = r.PathValue("batchID"), r.PathValue("remediationID")
	result, err := s.service.ChangeRemediation(r.Context(), command)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) ApproveHandler(w http.ResponseWriter, r *http.Request) {
	var command application.ApproveCommand
	if err := decodeJSON(w, r, &command); err != nil {
		handleError(w, err)
		return
	}
	command.BatchID = r.PathValue("batchID")
	result, err := s.service.Approve(r.Context(), command)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) VerifyPermitsHandler(w http.ResponseWriter, r *http.Request) {
	result, err := s.service.VerifyPermits(r.Context())
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) VerifyPermitHandler(w http.ResponseWriter, r *http.Request) {
	var sequence int64
	var err error
	if value := r.URL.Query().Get("sequence"); value != "" {
		sequence, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			handleError(w, domain.Validation("invalid_permit_sequence", "sequence 必须是正整数"))
			return
		}
	}
	result, err := s.service.VerifyPermit(r.Context(), sequence, r.URL.Query().Get("permitDigest"))
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
