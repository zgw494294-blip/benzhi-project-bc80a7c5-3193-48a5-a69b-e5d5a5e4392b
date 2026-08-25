package httpapi

import (
	"net/http"

	"scenicpermit/internal/application"
	"scenicpermit/internal/domain"
)

func (s *Server) HealthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "scenicpermit"})
}

func (s *Server) ListBatchesHandler(w http.ResponseWriter, r *http.Request) {
	batches, err := s.service.ListBatches(r.Context())
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": batches})
}

func (s *Server) CreateBatchHandler(w http.ResponseWriter, r *http.Request) {
	var command application.CreateBatchCommand
	if err := decodeJSON(w, r, &command); err != nil {
		handleError(w, err)
		return
	}
	result, err := s.service.CreateBatch(r.Context(), command)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) BatchDetailHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter := application.BatchDetailFilter{Progress: domain.ProgressFilter{
		StageZone: query.Get("stageZone"), MaterialClass: query.Get("materialClass"), CheckCode: query.Get("checkCode"), Inspector: query.Get("inspector"), Status: query.Get("status"),
	}, RemediationOwner: query.Get("remediationOwner"), RemediationStatus: query.Get("remediationStatus"), DueRisk: query.Get("dueRisk")}
	detail, err := s.service.BatchDetailFiltered(r.Context(), r.PathValue("batchID"), filter)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) UpdateUnitHandler(w http.ResponseWriter, r *http.Request) {
	var command application.UpdateUnitCommand
	if err := decodeJSON(w, r, &command); err != nil {
		handleError(w, err)
		return
	}
	command.BatchID, command.UnitID = r.PathValue("batchID"), r.PathValue("unitID")
	result, err := s.service.UpdateUnit(r.Context(), command)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) RemoveUnitHandler(w http.ResponseWriter, r *http.Request) {
	var command application.RemoveUnitCommand
	if err := decodeJSON(w, r, &command); err != nil {
		handleError(w, err)
		return
	}
	command.BatchID, command.UnitID = r.PathValue("batchID"), r.PathValue("unitID")
	result, err := s.service.RemoveUnit(r.Context(), command)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) PreflightPlanHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Definitions []domain.CheckDefinition `json:"checkDefinitions"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		handleError(w, err)
		return
	}
	view, err := s.service.PreflightPlan(r.Context(), r.PathValue("batchID"), body.Definitions)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *Server) UpdateBatchHandler(w http.ResponseWriter, r *http.Request) {
	var command application.UpdateBatchCommand
	if err := decodeJSON(w, r, &command); err != nil {
		handleError(w, err)
		return
	}
	command.BatchID = r.PathValue("batchID")
	result, err := s.service.UpdateBatch(r.Context(), command)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) AddUnitHandler(w http.ResponseWriter, r *http.Request) {
	var command application.AddUnitCommand
	if err := decodeJSON(w, r, &command); err != nil {
		handleError(w, err)
		return
	}
	command.BatchID = r.PathValue("batchID")
	result, err := s.service.AddUnit(r.Context(), command)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) SubmitPlanHandler(w http.ResponseWriter, r *http.Request) {
	var command application.SubmitPlanCommand
	if err := decodeJSON(w, r, &command); err != nil {
		handleError(w, err)
		return
	}
	command.BatchID = r.PathValue("batchID")
	result, err := s.service.SubmitPlan(r.Context(), command)
	if err != nil {
		handleError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
