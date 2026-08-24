package domain

import (
	"strings"
	"time"
)

type ReviewFinding struct {
	ID                   string        `json:"id"`
	CaseID               string        `json:"caseId"`
	RevisionID           string        `json:"revisionId"`
	RuleCode             string        `json:"ruleCode"`
	RuleVersion          string        `json:"ruleVersion"`
	Severity             Severity      `json:"severity"`
	LocationRef          string        `json:"locationRef"`
	Message              string        `json:"message"`
	Status               FindingStatus `json:"status"`
	ResolutionNote       string        `json:"resolutionNote,omitempty"`
	ResolvedByRevisionID string        `json:"resolvedByRevisionId,omitempty"`
	CreatedAt            time.Time     `json:"createdAt"`
	ResolvedAt           *time.Time    `json:"resolvedAt,omitempty"`
}

func NewFinding(id, caseID, revisionID, ruleCode, ruleVersion string, severity Severity, location, message string, now time.Time) (ReviewFinding, error) {
	if id == "" || caseID == "" || revisionID == "" || ruleCode == "" || ruleVersion == "" || strings.TrimSpace(message) == "" {
		return ReviewFinding{}, invalid("invalid_finding", "发现项的标识、规则和消息不能为空")
	}
	if severity != SeverityInfo && severity != SeverityWarning && severity != SeverityBlocker {
		return ReviewFinding{}, invalid("invalid_severity", "发现项严重级别无效")
	}
	return ReviewFinding{ID: id, CaseID: caseID, RevisionID: revisionID, RuleCode: ruleCode, RuleVersion: ruleVersion, Severity: severity, LocationRef: location, Message: message, Status: FindingOpen, CreatedAt: now.UTC()}, nil
}

func (f *ReviewFinding) Resolve(note, revisionID string, now time.Time) error {
	if f.Status != FindingOpen {
		return invalid("finding_already_resolved", "发现项 %s 已关闭", f.ID)
	}
	if strings.TrimSpace(note) == "" || strings.TrimSpace(revisionID) == "" || revisionID == f.RevisionID {
		return invalid("invalid_resolution", "处置说明必须关联后继修订")
	}
	t := now.UTC()
	f.Status = FindingResolved
	f.ResolutionNote = strings.TrimSpace(note)
	f.ResolvedByRevisionID = revisionID
	f.ResolvedAt = &t
	return nil
}
