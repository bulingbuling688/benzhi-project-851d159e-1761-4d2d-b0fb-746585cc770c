package httpapi

import (
	"net/http"
	"surveyrelease/internal/application"
)

func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) CreateCase(w http.ResponseWriter, r *http.Request) {
	var in application.CreateCaseInput
	if err := decodeStrict(w, r, &in); err != nil {
		writeProblem(w, r, err)
		return
	}
	meta, err := parseMeta(r, false)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	raw, version, err := a.service.CreateCase(r.Context(), meta, in)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	writeRaw(w, http.StatusCreated, raw, version)
}

func (a *API) SubmitCalibration(w http.ResponseWriter, r *http.Request) {
	var in application.CalibrationInput
	if err := decodeStrict(w, r, &in); err != nil {
		writeProblem(w, r, err)
		return
	}
	meta, err := parseMeta(r, true)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	raw, version, err := a.service.SubmitCalibration(r.Context(), r.PathValue("caseID"), meta, in)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	writeRaw(w, http.StatusOK, raw, version)
}

func (a *API) ValidateCase(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > 0 {
		var empty struct{}
		if err := decodeStrict(w, r, &empty); err != nil {
			writeProblem(w, r, err)
			return
		}
	}
	meta, err := parseMeta(r, true)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	raw, version, err := a.service.Validate(r.Context(), r.PathValue("caseID"), meta)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	writeRaw(w, http.StatusOK, raw, version)
}

func (a *API) RemediateCase(w http.ResponseWriter, r *http.Request) {
	var in application.RemediationInput
	if err := decodeStrict(w, r, &in); err != nil {
		writeProblem(w, r, err)
		return
	}
	meta, err := parseMeta(r, true)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	raw, version, err := a.service.Remediate(r.Context(), r.PathValue("caseID"), meta, in)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	writeRaw(w, http.StatusOK, raw, version)
}

func (a *API) ReviewCase(w http.ResponseWriter, r *http.Request) {
	var in application.ReviewInput
	if err := decodeStrict(w, r, &in); err != nil {
		writeProblem(w, r, err)
		return
	}
	meta, err := parseMeta(r, true)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	raw, version, err := a.service.Review(r.Context(), r.PathValue("caseID"), meta, in)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	writeRaw(w, http.StatusOK, raw, version)
}

func (a *API) FreezeCase(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > 0 {
		var empty struct{}
		if err := decodeStrict(w, r, &empty); err != nil {
			writeProblem(w, r, err)
			return
		}
	}
	meta, err := parseMeta(r, true)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	raw, version, err := a.service.Freeze(r.Context(), r.PathValue("caseID"), meta)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	writeRaw(w, http.StatusOK, raw, version)
}

func (a *API) IssueCredential(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > 0 {
		var empty struct{}
		if err := decodeStrict(w, r, &empty); err != nil {
			writeProblem(w, r, err)
			return
		}
	}
	meta, err := parseMeta(r, true)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	raw, version, err := a.service.Issue(r.Context(), r.PathValue("caseID"), meta)
	if err != nil {
		writeProblem(w, r, err)
		return
	}
	writeRaw(w, http.StatusCreated, raw, version)
}
