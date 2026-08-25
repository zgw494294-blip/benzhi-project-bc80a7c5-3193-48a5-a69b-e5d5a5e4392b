package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"scenicpermit/internal/domain"
)

const maxRequestBytes = 1 << 20

type errorBody struct {
	Error apiError `json:"error"`
}
type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: apiError{Code: code, Message: message}})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	if contentType := r.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(contentType, "application/json") {
		return domain.Validation("unsupported_content_type", "请求必须使用 application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return domain.Validation("body_too_large", "请求体超过 1 MiB 限制")
		}
		return domain.Validation("invalid_json", "JSON 请求格式无效："+err.Error())
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.Validation("multiple_json_values", "请求体只能包含一个 JSON 对象")
	}
	return nil
}

func handleError(w http.ResponseWriter, err error) {
	if rule, ok := domain.AsRuleError(err); ok {
		switch rule.Kind {
		case domain.KindValidation:
			writeError(w, http.StatusBadRequest, rule.Code, rule.Message)
		case domain.KindNotFound:
			writeError(w, http.StatusNotFound, rule.Code, rule.Message)
		case domain.KindConflict, domain.KindImmutable:
			writeError(w, http.StatusConflict, rule.Code, rule.Message)
		default:
			writeError(w, http.StatusBadRequest, rule.Code, rule.Message)
		}
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "服务处理请求失败")
}
