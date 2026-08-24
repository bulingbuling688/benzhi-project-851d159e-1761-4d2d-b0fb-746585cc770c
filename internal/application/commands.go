package application

import (
	"surveyrelease/internal/domain"
	"time"
)

type CommandMeta struct {
	Actor           Actor
	IdempotencyKey  string
	ExpectedVersion *int64
}

type CreateCaseInput struct {
	Title               string            `json:"title"`
	AcquisitionBatch    string            `json:"acquisitionBatch"`
	CoordinateReference string            `json:"coordinateReference"`
	ScopeDescription    string            `json:"scopeDescription"`
	Artifacts           []domain.Artifact `json:"artifacts"`
}

type CalibrationInput struct {
	CoveragePercent   float64                  `json:"coveragePercent"`
	HorizontalErrorCM float64                  `json:"horizontalErrorCm"`
	VerticalErrorCM   float64                  `json:"verticalErrorCm"`
	Evidence          CalibrationEvidenceInput `json:"calibrationEvidence"`
}

type CalibrationEvidenceInput struct {
	Reference       string    `json:"reference"`
	Instrument      string    `json:"instrument"`
	CalibratedAt    time.Time `json:"calibratedAt"`
	CertificateHash string    `json:"certificateHash"`
}

func (in CalibrationEvidenceInput) DomainEvidence() domain.CalibrationEvidence {
	return domain.CalibrationEvidence{Reference: in.Reference, Instrument: in.Instrument, CalibratedAt: in.CalibratedAt, CertificateHash: in.CertificateHash}
}

type RemediationInput struct {
	Artifacts   []domain.Artifact `json:"artifacts"`
	Resolutions map[string]string `json:"resolutions"`
}

type ReviewInput struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`
}

type VerifyInput struct {
	CredentialNumber string `json:"credentialNumber"`
	VerificationCode string `json:"verificationCode"`
	ManifestHash     string `json:"manifestHash"`
}

type CaseResponse struct {
	Case            *domain.ReleaseCase     `json:"case"`
	Version         int64                   `json:"version"`
	ReviewReadiness *domain.ReviewReadiness `json:"reviewReadiness,omitempty"`
}

type CredentialResponse struct {
	Credential *domain.ReleaseCredential `json:"credential"`
	Version    int64                     `json:"version"`
}

type VerificationResult struct {
	Valid            bool      `json:"valid"`
	CaseID           string    `json:"caseId,omitempty"`
	CredentialNumber string    `json:"credentialNumber,omitempty"`
	ManifestHash     string    `json:"manifestHash,omitempty"`
	IssuedAt         time.Time `json:"issuedAt,omitempty"`
	Reason           string    `json:"reason,omitempty"`
}
