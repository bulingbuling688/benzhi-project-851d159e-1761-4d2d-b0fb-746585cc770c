package domain

import "time"

type Status string

const (
	StatusDraft               Status = "draft"
	StatusValidating          Status = "validating"
	StatusRemediationRequired Status = "remediation_required"
	StatusReviewing           Status = "reviewing"
	StatusApproved            Status = "approved"
	StatusFrozen              Status = "frozen"
	StatusReleased            Status = "released"
)

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityBlocker Severity = "blocker"
)

type FindingStatus string

const (
	FindingOpen     FindingStatus = "open"
	FindingResolved FindingStatus = "resolved"
)

type Artifact struct {
	Path         string `json:"path"`
	SHA256       string `json:"sha256"`
	SizeBytes    int64  `json:"sizeBytes"`
	SensitiveTag bool   `json:"sensitiveTag,omitempty"`
}

type CalibrationEvidence struct {
	Reference           string    `json:"reference"`
	Instrument          string    `json:"instrument"`
	CalibratedAt        time.Time `json:"calibratedAt"`
	CertificateHash     string    `json:"certificateHash"`
	ValidUntil          time.Time `json:"validUntil"`
	ValidationStatus    string    `json:"validationStatus"`
	EvidenceFingerprint string    `json:"evidenceFingerprint"`
}
