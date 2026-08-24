package httpapi

import (
	"encoding/json"
	"net/http"

	"surveyrelease/internal/application"
)

type problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail"`
	Code      string `json:"code"`
	RequestID string `json:"requestId,omitempty"`
}

func writeProblem(w http.ResponseWriter, r *http.Request, err error) {
	e := application.Classify(err)
	status := http.StatusInternalServerError
	title := "服务内部错误"
	switch e.Kind {
	case application.KindValidation:
		status = http.StatusBadRequest
		title = "请求字段无效"
	case application.KindUnauthorized:
		status = http.StatusForbidden
		title = "角色无权操作"
	case application.KindNotFound:
		status = http.StatusNotFound
		title = "资源不存在"
	case application.KindConflict:
		status = http.StatusConflict
		title = "版本或幂等冲突"
	case application.KindState:
		status = http.StatusUnprocessableEntity
		title = "业务状态不允许操作"
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{Type: "https://survey-release.local/problems/" + e.Code, Title: title, Status: status, Detail: e.Message, Code: e.Code, RequestID: requestIDFrom(r.Context())})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeRaw(w http.ResponseWriter, status int, raw json.RawMessage, version int64) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("ETag", formatVersion(version))
	w.WriteHeader(status)
	_, _ = w.Write(append(raw, '\n'))
}
