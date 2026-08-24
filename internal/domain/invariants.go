package domain

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// ValidateIntegrity 检查从账本恢复的聚合是否仍满足领域不变量。
func (c *ReleaseCase) ValidateIntegrity() error {
	if c == nil {
		return invalid("nil_case", "档案不能为空")
	}
	if strings.TrimSpace(c.ID) == "" || strings.TrimSpace(c.Title) == "" {
		return invalid("invalid_case_identity", "档案 ID 和标题不能为空")
	}
	if c.Version < 1 {
		return invalid("invalid_case_version", "档案版本必须为正数")
	}
	if c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() || c.UpdatedAt.Before(c.CreatedAt) {
		return invalid("invalid_case_time", "档案创建或更新时间无效")
	}
	if !validStatus(c.Status) {
		return invalid("invalid_case_status", "未知档案状态 %q", c.Status)
	}
	if len(c.Revisions) == 0 {
		return invalid("missing_revisions", "档案至少需要一个修订")
	}
	revisions := make(map[string]DatasetRevision, len(c.Revisions))
	for i, revision := range c.Revisions {
		if err := validateRevisionIntegrity(c.ID, revision, i, c.Revisions); err != nil {
			return err
		}
		if _, exists := revisions[revision.ID]; exists {
			return invalid("duplicate_revision", "修订 ID 重复: %s", revision.ID)
		}
		revisions[revision.ID] = revision
	}
	certificates := map[string]CalibrationEvidence{}
	for _, revision := range c.Revisions {
		if revision.CalibrationEvidence == nil {
			continue
		}
		registered, exists := certificates[revision.CalibrationEvidence.CertificateHash]
		if exists && !sameCertificateIdentity(registered, *revision.CalibrationEvidence) {
			return invalid("calibration_evidence_conflict", "相同 certificateHash 的证据谱系描述不一致")
		}
		certificates[revision.CalibrationEvidence.CertificateHash] = *revision.CalibrationEvidence
	}
	current, exists := revisions[c.CurrentRevisionID]
	if !exists {
		return invalid("invalid_current_revision", "当前修订不属于档案")
	}
	if current.RevisionNumber != len(c.Revisions) {
		return invalid("stale_current_revision", "当前修订必须是谱系末端")
	}
	findings := make(map[string]struct{}, len(c.Findings))
	for _, finding := range c.Findings {
		if err := validateFindingIntegrity(c.ID, finding, revisions); err != nil {
			return err
		}
		if _, exists := findings[finding.ID]; exists {
			return invalid("duplicate_finding", "发现项 ID 重复: %s", finding.ID)
		}
		findings[finding.ID] = struct{}{}
	}
	if err := c.validateValidationBatches(revisions); err != nil {
		return err
	}
	if err := c.validateReviewDecisions(revisions); err != nil {
		return err
	}
	return c.validateStateContent(current)
}

func validStatus(status Status) bool {
	switch status {
	case StatusDraft, StatusValidating, StatusRemediationRequired, StatusReviewing, StatusApproved, StatusFrozen, StatusReleased:
		return true
	default:
		return false
	}
}

func validateRevisionIntegrity(caseID string, revision DatasetRevision, index int, all []DatasetRevision) error {
	if revision.ID == "" || revision.CaseID != caseID {
		return invalid("invalid_revision_owner", "修订缺少 ID 或不属于档案")
	}
	if revision.RevisionNumber != index+1 {
		return invalid("invalid_revision_number", "修订 %s 的 revisionNumber 不连续", revision.ID)
	}
	if index == 0 && revision.ParentRevisionID != "" {
		return invalid("invalid_revision_lineage", "首个修订不能包含父修订")
	}
	if index > 0 && revision.ParentRevisionID != all[index-1].ID {
		return invalid("invalid_revision_lineage", "修订 %s 未继承上一修订", revision.ID)
	}
	if revision.SubmittedBy == "" || revision.CreatedAt.IsZero() {
		return invalid("invalid_revision_metadata", "修订 %s 缺少提交者或创建时间", revision.ID)
	}
	if len(revision.Artifacts) == 0 {
		return invalid("missing_artifacts", "修订 %s 没有成果文件", revision.ID)
	}
	prepared, contentHash, summary, err := prepareArtifacts(revision.Artifacts)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(prepared, revision.Artifacts) || contentHash != revision.RevisionContentHash || !reflect.DeepEqual(summary, revision.RegistrationSummary) {
		return invalid("revision_summary_mismatch", "修订 %s 的内容指纹或登记摘要与成果清单不一致", revision.ID)
	}
	if revision.CalibrationEvidence != nil && !revision.HasCompleteCalibration() {
		return invalid("partial_calibration", "修订 %s 的校准信息不完整", revision.ID)
	}
	if revision.CalibrationEvidence != nil {
		if err := validateCalibrationIntegrity(*revision.CalibrationEvidence); err != nil {
			return err
		}
	}
	if index == 0 && (revision.RevisionDiff != nil || len(revision.BlockerResolutionLinks) > 0) {
		return invalid("invalid_initial_revision_diff", "首个修订不能包含整改差异或处置关联")
	}
	if index > 0 {
		if revision.RevisionDiff == nil || !reflect.DeepEqual(*revision.RevisionDiff, CompareRevisions(all[index-1], revision)) {
			return invalid("revision_diff_mismatch", "修订 %s 的文件差异与父修订不一致", revision.ID)
		}
		for _, link := range revision.BlockerResolutionLinks {
			if link.FindingID == "" || link.ResolutionNote == "" || link.ResolvedByRevisionID != revision.ID || (link.Status != "pending_revalidation" && link.Status != "resolved_by_file_change") {
				return invalid("invalid_blocker_resolution_link", "修订 %s 包含无效处置关联", revision.ID)
			}
		}
	}
	return nil
}

func (c *ReleaseCase) validateValidationBatches(revisions map[string]DatasetRevision) error {
	seen := map[string]struct{}{}
	var previous []ReviewFinding
	for _, batch := range c.ValidationBatches {
		revision, exists := revisions[batch.RevisionID]
		if !exists || batch.CaseID != c.ID || batch.RuleVersion != CurrentRuleVersion || batch.ExecutedAt.IsZero() {
			return invalid("invalid_validation_batch", "核验批次 %s 的归属或规则元数据无效", batch.BatchID)
		}
		if _, duplicate := seen[batch.BatchID]; duplicate || batch.BatchID == "" {
			return invalid("duplicate_validation_batch", "核验批次 ID 重复或为空")
		}
		seen[batch.BatchID] = struct{}{}
		current := make([]ReviewFinding, 0)
		for _, finding := range c.Findings {
			if finding.RevisionID == batch.RevisionID {
				current = append(current, finding)
			}
		}
		rebuilt, err := BuildValidationBatch(batch.BatchID, revision, current, previous, batch.ExecutedAt)
		if err != nil || !reflect.DeepEqual(rebuilt, batch) {
			return invalid("validation_batch_mismatch", "核验批次 %s 的摘要或指纹与发现项不一致", batch.BatchID)
		}
		previous = current
	}
	return nil
}

func (c *ReleaseCase) validateReviewDecisions(revisions map[string]DatasetRevision) error {
	seen := map[string]struct{}{}
	batches := map[string]ValidationBatch{}
	for _, batch := range c.ValidationBatches {
		batches[batch.BatchID] = batch
	}
	for _, decision := range c.ReviewDecisions {
		batch, exists := batches[decision.ValidationBatchID]
		if decision.ID == "" || decision.Actor == "" || decision.DecidedAt.IsZero() || (decision.Decision != "approve" && decision.Decision != "reject") {
			return invalid("invalid_review_decision", "审查决定缺少必要元数据")
		}
		if _, duplicate := seen[decision.ID]; duplicate {
			return invalid("duplicate_review_decision", "审查决定 ID 重复: %s", decision.ID)
		}
		seen[decision.ID] = struct{}{}
		if _, existsRevision := revisions[decision.CandidateRevisionID]; !existsRevision || !exists || batch.RevisionID != decision.CandidateRevisionID || decision.RuleVersion != batch.RuleVersion {
			return invalid("review_decision_context_mismatch", "审查决定 %s 的候选修订或核验批次上下文不一致", decision.ID)
		}
		if decision.Decision == "reject" && strings.TrimSpace(decision.Reason) == "" {
			return invalid("invalid_review_decision", "退回决定必须包含理由")
		}
		expectedWarnings := make([]ReviewWarning, 0)
		for _, finding := range c.Findings {
			if finding.RevisionID == decision.CandidateRevisionID && finding.Severity == SeverityWarning {
				expectedWarnings = append(expectedWarnings, ReviewWarning{FindingID: finding.ID, RuleCode: finding.RuleCode, LocationRef: finding.LocationRef, Message: finding.Message})
			}
		}
		sort.Slice(expectedWarnings, func(i, j int) bool { return expectedWarnings[i].FindingID < expectedWarnings[j].FindingID })
		if !reflect.DeepEqual(expectedWarnings, decision.Warnings) {
			return invalid("review_decision_context_mismatch", "审查决定 %s 的 warning 快照与候选修订不一致", decision.ID)
		}
	}
	return nil
}

func validateFindingIntegrity(caseID string, finding ReviewFinding, revisions map[string]DatasetRevision) error {
	if finding.ID == "" || finding.CaseID != caseID {
		return invalid("invalid_finding_owner", "发现项缺少 ID 或不属于档案")
	}
	if _, exists := revisions[finding.RevisionID]; !exists {
		return invalid("invalid_finding_revision", "发现项 %s 关联未知修订", finding.ID)
	}
	if finding.RuleCode == "" || finding.RuleVersion == "" || finding.Message == "" || finding.CreatedAt.IsZero() {
		return invalid("invalid_finding_metadata", "发现项 %s 的规则元数据不完整", finding.ID)
	}
	if finding.Severity != SeverityInfo && finding.Severity != SeverityWarning && finding.Severity != SeverityBlocker {
		return invalid("invalid_finding_severity", "发现项 %s 严重级别无效", finding.ID)
	}
	switch finding.Status {
	case FindingOpen:
		if finding.ResolvedAt != nil || finding.ResolvedByRevisionID != "" || finding.ResolutionNote != "" {
			return invalid("invalid_open_finding", "打开的发现项 %s 不能包含处置信息", finding.ID)
		}
	case FindingResolved:
		resolvedRevision, exists := revisions[finding.ResolvedByRevisionID]
		originRevision := revisions[finding.RevisionID]
		if !exists || resolvedRevision.RevisionNumber <= originRevision.RevisionNumber || finding.ResolvedAt == nil || finding.ResolutionNote == "" {
			return invalid("invalid_resolved_finding", "关闭的发现项 %s 必须关联后继修订和完整处置信息", finding.ID)
		}
	default:
		return invalid("invalid_finding_status", "发现项 %s 状态无效", finding.ID)
	}
	return nil
}

func (c *ReleaseCase) validateStateContent(current DatasetRevision) error {
	calibrated := current.HasCompleteCalibration()
	switch c.Status {
	case StatusDraft, StatusRemediationRequired:
		if c.Manifest != nil || c.Credential != nil {
			return invalid("premature_release_material", "未冻结档案不能包含清单或凭据")
		}
	case StatusValidating, StatusReviewing, StatusApproved:
		if !calibrated {
			return invalid("missing_calibration", "状态 %s 要求当前修订具有完整校准数据", c.Status)
		}
		if c.Manifest != nil || c.Credential != nil {
			return invalid("premature_release_material", "批准前档案不能包含清单或凭据")
		}
	case StatusFrozen:
		if c.Manifest == nil || c.ManifestHash == "" || c.Credential != nil {
			return invalid("invalid_frozen_content", "frozen 档案必须包含清单和哈希且不能已有凭据")
		}
	case StatusReleased:
		if c.Manifest == nil || c.ManifestHash == "" || c.Credential == nil {
			return invalid("invalid_released_content", "released 档案必须包含清单、哈希和凭据")
		}
		if c.Credential.CaseID != c.ID || c.Credential.RevisionID != c.CurrentRevisionID || c.Credential.ManifestHash != c.ManifestHash {
			return invalid("credential_mismatch", "released 档案的凭据与冻结内容不一致")
		}
	}
	if c.Status == StatusReviewing || c.Status == StatusApproved || c.Status == StatusFrozen || c.Status == StatusReleased {
		batch, exists := c.LatestValidationBatch()
		if !exists || batch.RevisionID != c.CurrentRevisionID || batch.RuleVersion != CurrentRuleVersion {
			return invalid("missing_current_validation_batch", "状态 %s 要求当前修订具有最新规则核验批次", c.Status)
		}
	}
	if c.Status == StatusApproved || c.Status == StatusFrozen || c.Status == StatusReleased {
		if len(c.ReviewDecisions) == 0 {
			return invalid("missing_review_decision", "状态 %s 要求存在批准决定快照", c.Status)
		}
		latest := c.ReviewDecisions[len(c.ReviewDecisions)-1]
		if latest.Decision != "approve" || latest.CandidateRevisionID != c.CurrentRevisionID {
			return invalid("review_decision_context_mismatch", "状态 %s 的最新审查决定不是当前修订批准", c.Status)
		}
	}
	if c.Status == StatusReviewing || c.Status == StatusApproved || c.Status == StatusFrozen || c.Status == StatusReleased {
		if blockers := c.OpenBlockers(); len(blockers) > 0 {
			return invalid("open_blocker", "状态 %s 不允许存在 %d 个打开的阻断发现项", c.Status, len(blockers))
		}
	}
	if c.Manifest != nil && c.Manifest.RevisionID != c.CurrentRevisionID {
		return invalid("manifest_revision_mismatch", fmt.Sprintf("冻结清单修订 %s 不是当前修订", c.Manifest.RevisionID))
	}
	if c.Manifest != nil {
		hash, err := c.Manifest.Hash()
		if err != nil {
			return invalid("manifest_hash_failure", "冻结清单无法计算哈希: %v", err)
		}
		if hash != c.ManifestHash {
			return invalid("manifest_hash_mismatch", "冻结清单内容与 manifestHash 不一致")
		}
	}
	return nil
}
