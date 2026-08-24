package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"surveyrelease/internal/application"
)

func (a *API) GetCase(w http.ResponseWriter, r *http.Request) {
	response, err := a.service.GetCaseResponse(r.Context(), r.PathValue("caseID"))
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	w.Header().Set("ETag", formatVersion(response.Version))
	writeJSON(w, http.StatusOK, response)
}
func (a *API) GetTimeline(w http.ResponseWriter, r *http.Request) {
	query := application.TimelineQuery{Limit: application.DefaultTimelineLimit, EventType: strings.TrimSpace(r.URL.Query().Get("eventType"))}
	if raw := strings.TrimSpace(r.URL.Query().Get("afterSequence")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			writeProblem(w, r, applicationError("invalid_after_sequence", "afterSequence 必须是非负整数"))
			return
		}
		query.AfterSequence = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > application.MaxTimelineLimit {
			writeProblem(w, r, applicationError("invalid_timeline_limit", "limit 必须是 1 到 200 之间的整数"))
			return
		}
		query.Limit = value
	}
	page, err := a.service.Timeline(r.Context(), r.PathValue("caseID"), query)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}
func (a *API) VerifyCredential(w http.ResponseWriter, r *http.Request) {
	var in application.VerifyInput
	if err := decodeStrict(w, r, &in); err != nil {
		writeProblem(w, r, err)
		return
	}
	result, err := a.service.Verify(r.Context(), r.PathValue("caseID"), in)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
