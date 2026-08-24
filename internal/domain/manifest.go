package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"time"
)

type FrozenFinding struct {
	ID                   string     `json:"id"`
	RuleCode             string     `json:"ruleCode"`
	Severity             Severity   `json:"severity"`
	ResolutionNote       string     `json:"resolutionNote"`
	ResolvedByRevisionID string     `json:"resolvedByRevisionId"`
	ResolvedAt           *time.Time `json:"resolvedAt"`
}

type FrozenManifest struct {
	SchemaVersion int                 `json:"schemaVersion"`
	CaseID        string              `json:"caseId"`
	RevisionID    string              `json:"revisionId"`
	Artifacts     []Artifact          `json:"artifacts"`
	Evidence      CalibrationEvidence `json:"calibrationEvidence"`
	Measurements  map[string]float64  `json:"measurements"`
	Findings      []FrozenFinding     `json:"closedFindings"`
	FrozenBy      string              `json:"frozenBy"`
	FrozenAt      time.Time           `json:"frozenAt"`
}

func (m FrozenManifest) Hash() (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func BuildManifest(c *ReleaseCase, actor string, now time.Time) (FrozenManifest, string, error) {
	if c.Status != StatusApproved {
		return FrozenManifest{}, "", invalid("invalid_state", "只有 approved 档案可以冻结")
	}
	r, ok := c.CurrentRevision()
	if !ok || !r.HasCompleteCalibration() {
		return FrozenManifest{}, "", invalid("incomplete_revision", "候选修订或校准证据不完整")
	}
	closed := make([]FrozenFinding, 0)
	for _, f := range c.Findings {
		if f.Status == FindingOpen && f.Severity == SeverityBlocker {
			return FrozenManifest{}, "", invalid("open_blocker", "存在未关闭的阻断发现项")
		}
		if f.Status == FindingResolved {
			closed = append(closed, FrozenFinding{ID: f.ID, RuleCode: f.RuleCode, Severity: f.Severity, ResolutionNote: f.ResolutionNote, ResolvedByRevisionID: f.ResolvedByRevisionID, ResolvedAt: f.ResolvedAt})
		}
	}
	sort.Slice(closed, func(i, j int) bool { return closed[i].ID < closed[j].ID })
	m := FrozenManifest{SchemaVersion: 1, CaseID: c.ID, RevisionID: r.ID, Artifacts: append([]Artifact(nil), r.Artifacts...), Evidence: *r.CalibrationEvidence, Measurements: map[string]float64{"coveragePercent": *r.CoveragePercent, "horizontalErrorCm": *r.HorizontalErrorCM, "verticalErrorCm": *r.VerticalErrorCM}, Findings: closed, FrozenBy: actor, FrozenAt: now.UTC()}
	hash, err := m.Hash()
	if err != nil {
		return FrozenManifest{}, "", err
	}
	return m, hash, nil
}
