package domain

import (
	"sort"
	"strings"
	"time"
)

type ReviewWarning struct {
	FindingID   string `json:"findingId"`
	RuleCode    string `json:"ruleCode"`
	LocationRef string `json:"locationRef"`
	Message     string `json:"message"`
}

type ReviewReadiness struct {
	Ready               bool            `json:"ready"`
	CandidateRevisionID string          `json:"candidateRevisionId"`
	ValidationBatchID   string          `json:"validationBatchId,omitempty"`
	RuleVersion         string          `json:"ruleVersion,omitempty"`
	UnmetItems          []string        `json:"unmetItems"`
	Warnings            []ReviewWarning `json:"warnings"`
}

type ReviewDecision struct {
	ID                  string          `json:"id"`
	Decision            string          `json:"decision"`
	CandidateRevisionID string          `json:"candidateRevisionId"`
	ValidationBatchID   string          `json:"validationBatchId"`
	RuleVersion         string          `json:"ruleVersion"`
	Warnings            []ReviewWarning `json:"warnings"`
	Actor               string          `json:"actor"`
	Reason              string          `json:"reason,omitempty"`
	DecidedAt           time.Time       `json:"decidedAt"`
}

func (c *ReleaseCase) ReviewReadinessAt(now time.Time) ReviewReadiness {
	result := ReviewReadiness{CandidateRevisionID: c.CurrentRevisionID, UnmetItems: []string{}, Warnings: []ReviewWarning{}}
	if c.Status != StatusReviewing {
		result.UnmetItems = append(result.UnmetItems, "case_not_reviewing")
	}
	batch, exists := c.LatestValidationBatch()
	if !exists || batch.RevisionID != c.CurrentRevisionID {
		result.UnmetItems = append(result.UnmetItems, "missing_current_validation_batch")
	} else {
		result.ValidationBatchID = batch.BatchID
		result.RuleVersion = batch.RuleVersion
		if batch.RuleVersion != CurrentRuleVersion {
			result.UnmetItems = append(result.UnmetItems, "rule_version_mismatch")
		}
	}
	revision, revisionExists := c.CurrentRevision()
	if !revisionExists || !revision.HasCompleteCalibration() {
		result.UnmetItems = append(result.UnmetItems, "missing_calibration_evidence")
	} else if now.UTC().After(revision.CalibrationEvidence.ValidUntil) {
		result.UnmetItems = append(result.UnmetItems, "calibration_evidence_expired")
	}
	if len(c.OpenBlockers()) > 0 {
		result.UnmetItems = append(result.UnmetItems, "open_blockers")
	}
	if exists && batch.RevisionID == c.CurrentRevisionID {
		for _, finding := range c.Findings {
			if finding.RevisionID == c.CurrentRevisionID && finding.Status == FindingOpen && finding.Severity == SeverityWarning {
				result.Warnings = append(result.Warnings, ReviewWarning{FindingID: finding.ID, RuleCode: finding.RuleCode, LocationRef: finding.LocationRef, Message: finding.Message})
			}
		}
	}
	sort.Slice(result.Warnings, func(i, j int) bool { return result.Warnings[i].FindingID < result.Warnings[j].FindingID })
	result.Ready = len(result.UnmetItems) == 0
	return result
}

func (c *ReleaseCase) Review(decisionID string, approve bool, reason, actor string, readiness ReviewReadiness, now time.Time) error {
	if c.Status != StatusReviewing {
		return invalid("invalid_state", "当前状态 %s 不能人工审查", c.Status)
	}
	actual := c.ReviewReadinessAt(now)
	if !actual.Ready || !readiness.Ready || actual.CandidateRevisionID != readiness.CandidateRevisionID || actual.ValidationBatchID != readiness.ValidationBatchID {
		return invalid("review_not_ready", "候选修订尚未满足审查就绪条件")
	}
	if strings.TrimSpace(decisionID) == "" || strings.TrimSpace(actor) == "" {
		return invalid("invalid_review_decision", "审查决定缺少标识或操作者")
	}
	reason = strings.TrimSpace(reason)
	decision := "approve"
	if approve {
		c.Status = StatusApproved
		c.ReviewNote = reason
	} else {
		if reason == "" {
			return invalid("missing_reason", "退回必须提供理由")
		}
		decision = "reject"
		c.Status = StatusRemediationRequired
		c.ReviewNote = reason
	}
	c.ReviewDecisions = append(c.ReviewDecisions, ReviewDecision{ID: decisionID, Decision: decision, CandidateRevisionID: actual.CandidateRevisionID, ValidationBatchID: actual.ValidationBatchID, RuleVersion: actual.RuleVersion, Warnings: append([]ReviewWarning{}, actual.Warnings...), Actor: strings.TrimSpace(actor), Reason: reason, DecidedAt: now.UTC()})
	c.Touch(now)
	return nil
}
