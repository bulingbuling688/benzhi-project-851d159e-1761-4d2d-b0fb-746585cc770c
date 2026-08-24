package httpapi

import (
	"log/slog"
	"net/http"

	"surveyrelease/internal/application"
)

type API struct {
	service *application.Service
	logger  *slog.Logger
}

func New(service *application.Service, logger *slog.Logger) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	api := &API{service: service, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.Health)
	mux.HandleFunc("POST /api/v1/release-cases", api.CreateCase)
	mux.HandleFunc("GET /api/v1/release-cases/{caseID}", api.GetCase)
	mux.HandleFunc("POST /api/v1/release-cases/{caseID}/calibration", api.SubmitCalibration)
	mux.HandleFunc("POST /api/v1/release-cases/{caseID}/validation", api.ValidateCase)
	mux.HandleFunc("POST /api/v1/release-cases/{caseID}/remediations", api.RemediateCase)
	mux.HandleFunc("POST /api/v1/release-cases/{caseID}/review", api.ReviewCase)
	mux.HandleFunc("POST /api/v1/release-cases/{caseID}/freeze", api.FreezeCase)
	mux.HandleFunc("POST /api/v1/release-cases/{caseID}/credentials", api.IssueCredential)
	mux.HandleFunc("GET /api/v1/release-cases/{caseID}/timeline", api.GetTimeline)
	mux.HandleFunc("POST /api/v1/release-cases/{caseID}/credential-verification", api.VerifyCredential)
	return accessLog(logger, recoverPanic(logger, mux))
}
