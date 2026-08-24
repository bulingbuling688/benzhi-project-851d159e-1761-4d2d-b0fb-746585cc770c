package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"time"
)

const CurrentRuleVersion = "survey-public-release-rules/1.0.0"

type SeverityCounts struct {
	Blocker int `json:"blocker"`
	Warning int `json:"warning"`
	Info    int `json:"info"`
}

type FindingDifferenceGroup struct {
	FindingKeys []string       `json:"findingKeys"`
	Counts      SeverityCounts `json:"counts"`
}

type FindingDifferenceSummary struct {
	Added      FindingDifferenceGroup `json:"added"`
	Persistent FindingDifferenceGroup `json:"persistent"`
	Resolved   FindingDifferenceGroup `json:"resolved"`
}

type ValidationBatch struct {
	BatchID               string                   `json:"batchId"`
	CaseID                string                   `json:"caseId"`
	RevisionID            string                   `json:"revisionId"`
	RuleVersion           string                   `json:"ruleVersion"`
	ExecutedAt            time.Time                `json:"executedAt"`
	SeverityCounts        SeverityCounts           `json:"severityCounts"`
	InputFingerprint      string                   `json:"inputFingerprint"`
	ValidationFingerprint string                   `json:"validationFingerprint"`
	DifferenceSummary     FindingDifferenceSummary `json:"differenceSummary"`
}

type validationFindingMaterial struct {
	RuleCode    string   `json:"ruleCode"`
	LocationRef string   `json:"locationRef"`
	Severity    Severity `json:"severity"`
	Message     string   `json:"message"`
}

type validationFingerprintMaterial struct {
	RevisionContentHash string                      `json:"revisionContentHash"`
	EvidenceFingerprint string                      `json:"evidenceFingerprint"`
	RuleVersion         string                      `json:"ruleVersion"`
	Findings            []validationFindingMaterial `json:"findings"`
}

type validationInputMaterial struct {
	RevisionContentHash string `json:"revisionContentHash"`
	EvidenceFingerprint string `json:"evidenceFingerprint"`
	RuleVersion         string `json:"ruleVersion"`
}

func findingKey(f ReviewFinding) string { return f.RuleCode + "\n" + f.LocationRef }

func addSeverity(counts *SeverityCounts, severity Severity) {
	switch severity {
	case SeverityBlocker:
		counts.Blocker++
	case SeverityWarning:
		counts.Warning++
	case SeverityInfo:
		counts.Info++
	}
}

func BuildValidationBatch(batchID string, revision DatasetRevision, findings, previous []ReviewFinding, now time.Time) (ValidationBatch, error) {
	if batchID == "" || !revision.HasCompleteCalibration() {
		return ValidationBatch{}, invalid("invalid_validation_batch", "核验批次缺少标识或完整校准证据")
	}
	ordered := append([]ReviewFinding(nil), findings...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].RuleCode == ordered[j].RuleCode {
			return ordered[i].LocationRef < ordered[j].LocationRef
		}
		return ordered[i].RuleCode < ordered[j].RuleCode
	})
	materialFindings := make([]validationFindingMaterial, 0, len(ordered))
	counts := SeverityCounts{}
	currentByKey := make(map[string]ReviewFinding, len(ordered))
	for _, finding := range ordered {
		if finding.CaseID != revision.CaseID || finding.RevisionID != revision.ID || finding.RuleVersion != CurrentRuleVersion {
			return ValidationBatch{}, invalid("invalid_validation_batch", "核验发现项不属于当前修订或规则版本不一致")
		}
		key := findingKey(finding)
		if _, exists := currentByKey[key]; exists {
			return ValidationBatch{}, invalid("duplicate_validation_finding", "核验批次包含重复规则位置: %s", key)
		}
		currentByKey[key] = finding
		addSeverity(&counts, finding.Severity)
		materialFindings = append(materialFindings, validationFindingMaterial{RuleCode: finding.RuleCode, LocationRef: finding.LocationRef, Severity: finding.Severity, Message: finding.Message})
	}
	previousByKey := make(map[string]ReviewFinding, len(previous))
	for _, finding := range previous {
		previousByKey[findingKey(finding)] = finding
	}
	difference := FindingDifferenceSummary{}
	for key, finding := range currentByKey {
		group := &difference.Added
		if _, exists := previousByKey[key]; exists {
			group = &difference.Persistent
		}
		group.FindingKeys = append(group.FindingKeys, key)
		addSeverity(&group.Counts, finding.Severity)
	}
	for key, finding := range previousByKey {
		if _, exists := currentByKey[key]; exists {
			continue
		}
		difference.Resolved.FindingKeys = append(difference.Resolved.FindingKeys, key)
		addSeverity(&difference.Resolved.Counts, finding.Severity)
	}
	sort.Strings(difference.Added.FindingKeys)
	sort.Strings(difference.Persistent.FindingKeys)
	sort.Strings(difference.Resolved.FindingKeys)
	input := validationInputMaterial{RevisionContentHash: revision.RevisionContentHash, EvidenceFingerprint: revision.CalibrationEvidence.EvidenceFingerprint, RuleVersion: CurrentRuleVersion}
	inputEncoded, err := json.Marshal(input)
	if err != nil {
		return ValidationBatch{}, err
	}
	inputSum := sha256.Sum256(inputEncoded)
	fingerprintInput := validationFingerprintMaterial{RevisionContentHash: input.RevisionContentHash, EvidenceFingerprint: input.EvidenceFingerprint, RuleVersion: input.RuleVersion, Findings: materialFindings}
	encoded, err := json.Marshal(fingerprintInput)
	if err != nil {
		return ValidationBatch{}, err
	}
	sum := sha256.Sum256(encoded)
	return ValidationBatch{BatchID: batchID, CaseID: revision.CaseID, RevisionID: revision.ID, RuleVersion: CurrentRuleVersion, ExecutedAt: now.UTC(), SeverityCounts: counts, InputFingerprint: hex.EncodeToString(inputSum[:]), ValidationFingerprint: hex.EncodeToString(sum[:]), DifferenceSummary: difference}, nil
}

func (c *ReleaseCase) LatestValidationBatch() (*ValidationBatch, bool) {
	if len(c.ValidationBatches) == 0 {
		return nil, false
	}
	return &c.ValidationBatches[len(c.ValidationBatches)-1], true
}

func (c *ReleaseCase) PreviousValidatedFindings() []ReviewFinding {
	for i := len(c.ValidationBatches) - 1; i >= 0; i-- {
		batch := c.ValidationBatches[i]
		if batch.RevisionID == c.CurrentRevisionID {
			continue
		}
		items := make([]ReviewFinding, 0)
		for _, finding := range c.Findings {
			if finding.RevisionID == batch.RevisionID {
				items = append(items, finding)
			}
		}
		return items
	}
	return nil
}

func (c *ReleaseCase) ApplyValidation(batch ValidationBatch, findings []ReviewFinding, now time.Time) error {
	if c.Status != StatusValidating {
		return invalid("invalid_state", "当前状态 %s 不能执行核验", c.Status)
	}
	if batch.CaseID != c.ID || batch.RevisionID != c.CurrentRevisionID || batch.RuleVersion != CurrentRuleVersion {
		return invalid("invalid_validation_batch", "核验批次必须属于当前修订并使用固定规则版本")
	}
	current, ok := c.CurrentRevision()
	if !ok {
		return invalid("missing_revision", "当前候选修订不存在")
	}
	rebuilt, err := BuildValidationBatch(batch.BatchID, *current, findings, c.PreviousValidatedFindings(), batch.ExecutedAt)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(batch, rebuilt) {
		return invalid("validation_batch_mismatch", "核验批次摘要与业务输入不一致")
	}
	for i := range c.Findings {
		finding := &c.Findings[i]
		if finding.Status != FindingOpen || finding.RevisionID == c.CurrentRevisionID {
			continue
		}
		for _, revision := range c.Revisions {
			for _, link := range revision.BlockerResolutionLinks {
				if link.FindingID == finding.ID {
					if err := finding.Resolve(link.ResolutionNote, revision.ID, now); err != nil {
						return err
					}
				}
			}
		}
	}
	c.Findings = append(c.Findings, findings...)
	c.ValidationBatches = append(c.ValidationBatches, batch)
	c.Status = StatusReviewing
	if batch.SeverityCounts.Blocker > 0 {
		c.Status = StatusRemediationRequired
	}
	c.Touch(now)
	return nil
}
