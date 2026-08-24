package domain

import (
	"regexp"
	"strings"
	"time"
)

var sha256Pattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

type DatasetRevision struct {
	ID                     string                      `json:"id"`
	CaseID                 string                      `json:"caseId"`
	ParentRevisionID       string                      `json:"parentRevisionId,omitempty"`
	RevisionNumber         int                         `json:"revisionNumber"`
	Artifacts              []Artifact                  `json:"artifacts"`
	RevisionContentHash    string                      `json:"revisionContentHash"`
	RegistrationSummary    ArtifactRegistrationSummary `json:"registrationSummary"`
	CoveragePercent        *float64                    `json:"coveragePercent,omitempty"`
	HorizontalErrorCM      *float64                    `json:"horizontalErrorCm,omitempty"`
	VerticalErrorCM        *float64                    `json:"verticalErrorCm,omitempty"`
	CalibrationEvidence    *CalibrationEvidence        `json:"calibrationEvidence,omitempty"`
	RevisionDiff           *RevisionDiff               `json:"revisionDiff,omitempty"`
	BlockerResolutionLinks []BlockerResolutionLink     `json:"blockerResolutionLinks,omitempty"`
	SensitiveElementsOpen  int                         `json:"sensitiveElementsOpen"`
	SubmittedBy            string                      `json:"submittedBy"`
	CreatedAt              time.Time                   `json:"createdAt"`
}

func NewRevision(id, caseID, parentID, actor string, number int, artifacts []Artifact, now time.Time) (DatasetRevision, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(caseID) == "" {
		return DatasetRevision{}, invalid("invalid_revision", "修订 ID 和档案 ID 不能为空")
	}
	if number < 1 || len(artifacts) == 0 {
		return DatasetRevision{}, invalid("invalid_revision", "修订号必须为正数且至少登记一个成果文件")
	}
	copyArtifacts, contentHash, summary, err := prepareArtifacts(artifacts)
	if err != nil {
		return DatasetRevision{}, err
	}
	return DatasetRevision{ID: id, CaseID: caseID, ParentRevisionID: parentID, RevisionNumber: number, Artifacts: copyArtifacts, RevisionContentHash: contentHash, RegistrationSummary: summary, SubmittedBy: actor, CreatedAt: now.UTC()}, nil
}

func (r *DatasetRevision) SetCalibration(coverage, horizontal, vertical float64, evidence CalibrationEvidence, now time.Time) error {
	if coverage < 0 || coverage > 100 || horizontal < 0 || vertical < 0 {
		return invalid("invalid_measurement", "覆盖率必须在 0 到 100 之间，误差不能为负数")
	}
	prepared, err := prepareCalibrationEvidence(evidence, now)
	if err != nil {
		return err
	}
	r.CoveragePercent = ptr(coverage)
	r.HorizontalErrorCM = ptr(horizontal)
	r.VerticalErrorCM = ptr(vertical)
	r.CalibrationEvidence = &prepared
	return nil
}

func (r DatasetRevision) HasCompleteCalibration() bool {
	return r.CoveragePercent != nil && r.HorizontalErrorCM != nil && r.VerticalErrorCM != nil && r.CalibrationEvidence != nil
}

func ptr(v float64) *float64 { return &v }
