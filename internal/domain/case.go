package domain

import (
	"strings"
	"time"
)

type ReleaseCase struct {
	ID                  string             `json:"id"`
	Title               string             `json:"title"`
	AcquisitionBatch    string             `json:"acquisitionBatch"`
	CoordinateReference string             `json:"coordinateReference"`
	ScopeDescription    string             `json:"scopeDescription"`
	Status              Status             `json:"status"`
	CurrentRevisionID   string             `json:"currentRevisionId"`
	Version             int64              `json:"version"`
	Revisions           []DatasetRevision  `json:"revisions"`
	Findings            []ReviewFinding    `json:"findings"`
	ValidationBatches   []ValidationBatch  `json:"validationBatches"`
	ReviewDecisions     []ReviewDecision   `json:"reviewDecisions"`
	Manifest            *FrozenManifest    `json:"frozenManifest,omitempty"`
	ManifestHash        string             `json:"manifestHash,omitempty"`
	Credential          *ReleaseCredential `json:"credential,omitempty"`
	ReviewNote          string             `json:"reviewNote,omitempty"`
	CreatedAt           time.Time          `json:"createdAt"`
	UpdatedAt           time.Time          `json:"updatedAt"`
}

func NewCase(id, title, batch, crs, scope string, revision DatasetRevision, now time.Time) (*ReleaseCase, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(title) == "" || strings.TrimSpace(batch) == "" || strings.TrimSpace(crs) == "" || strings.TrimSpace(scope) == "" {
		return nil, invalid("invalid_case", "档案标题、采集批次、坐标参考和成果范围不能为空")
	}
	if revision.CaseID != id || revision.ParentRevisionID != "" || revision.RevisionNumber != 1 {
		return nil, invalid("invalid_initial_revision", "首个修订必须属于本档案且无父修订")
	}
	now = now.UTC()
	return &ReleaseCase{ID: id, Title: strings.TrimSpace(title), AcquisitionBatch: strings.TrimSpace(batch), CoordinateReference: strings.TrimSpace(crs), ScopeDescription: strings.TrimSpace(scope), Status: StatusDraft, CurrentRevisionID: revision.ID, Version: 1, Revisions: []DatasetRevision{revision}, Findings: []ReviewFinding{}, ValidationBatches: []ValidationBatch{}, ReviewDecisions: []ReviewDecision{}, CreatedAt: now, UpdatedAt: now}, nil
}

func (c *ReleaseCase) CurrentRevision() (*DatasetRevision, bool) {
	for i := range c.Revisions {
		if c.Revisions[i].ID == c.CurrentRevisionID {
			return &c.Revisions[i], true
		}
	}
	return nil, false
}

func (c *ReleaseCase) Touch(now time.Time) { c.Version++; c.UpdatedAt = now.UTC() }

func (c *ReleaseCase) OpenBlockers() []ReviewFinding {
	items := []ReviewFinding{}
	for _, f := range c.Findings {
		if f.RevisionID == c.CurrentRevisionID && f.Status == FindingOpen && f.Severity == SeverityBlocker {
			items = append(items, f)
		}
	}
	return items
}
