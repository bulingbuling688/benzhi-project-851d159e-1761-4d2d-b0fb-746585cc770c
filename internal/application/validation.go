package application

import (
	"math"
	"regexp"
	"strings"
	"time"

	"surveyrelease/internal/domain"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
var hexHashPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

const DefaultTimelineLimit = 50
const MaxTimelineLimit = 200

var timelineEventTypes = map[string]struct{}{
	"case.created": {}, "calibration.submitted": {}, "validation.completed": {},
	"remediation.submitted": {}, "review.approve": {}, "review.reject": {},
	"case.frozen": {}, "credential.issued": {},
}

func validateCaseID(caseID string) error {
	if !identifierPattern.MatchString(caseID) {
		return fail(KindValidation, "invalid_case_id", "档案 ID 格式无效")
	}
	return nil
}

func validateCreateInput(in CreateCaseInput) error {
	fields := []struct {
		name, value string
		limit       int
	}{{"title", in.Title, 200}, {"acquisitionBatch", in.AcquisitionBatch, 120}, {"coordinateReference", in.CoordinateReference, 200}, {"scopeDescription", in.ScopeDescription, 2000}}
	for _, field := range fields {
		value := strings.TrimSpace(field.value)
		if value == "" {
			return fail(KindValidation, "missing_"+field.name, "%s 不能为空", field.name)
		}
		if len([]rune(value)) > field.limit {
			return fail(KindValidation, "field_too_long", "%s 长度不能超过 %d 个字符", field.name, field.limit)
		}
	}
	return validateArtifacts(in.Artifacts)
}

func validateArtifacts(artifacts []domain.Artifact) error {
	if len(artifacts) == 0 {
		return fail(KindValidation, "missing_artifacts", "至少需要一个成果文件")
	}
	if len(artifacts) > 10000 {
		return fail(KindValidation, "too_many_artifacts", "单个修订最多登记 10000 个成果文件")
	}
	seen := make(map[string]struct{}, len(artifacts))
	for _, artifact := range artifacts {
		path := strings.TrimSpace(artifact.Path)
		if path == "" || len([]rune(path)) > 1024 {
			return fail(KindValidation, "invalid_artifact_path", "成果文件 path 不能为空且不能超过 1024 个字符")
		}
		if strings.HasPrefix(path, "/") || strings.Contains(path, "../") {
			return fail(KindValidation, "unsafe_artifact_path", "成果文件 path 必须是安全的相对逻辑路径")
		}
		if !hexHashPattern.MatchString(strings.TrimSpace(artifact.SHA256)) {
			return fail(KindValidation, "invalid_artifact_hash", "成果文件 sha256 必须是 64 位十六进制字符串")
		}
		if artifact.SizeBytes < 0 {
			return fail(KindValidation, "invalid_artifact_size", "成果文件 sizeBytes 不能为负数")
		}
		if _, ok := seen[path]; ok {
			return fail(KindValidation, "duplicate_artifact", "成果文件 path 重复: %s", path)
		}
		seen[path] = struct{}{}
	}
	return nil
}

func validateCalibrationInput(in CalibrationInput, now time.Time) error {
	if math.IsNaN(in.CoveragePercent) || math.IsInf(in.CoveragePercent, 0) || in.CoveragePercent < 0 || in.CoveragePercent > 100 {
		return fail(KindValidation, "invalid_coverage", "coveragePercent 必须是 0 到 100 之间的有限数值")
	}
	if math.IsNaN(in.HorizontalErrorCM) || math.IsInf(in.HorizontalErrorCM, 0) || in.HorizontalErrorCM < 0 {
		return fail(KindValidation, "invalid_horizontal_error", "horizontalErrorCm 必须是非负有限数值")
	}
	if math.IsNaN(in.VerticalErrorCM) || math.IsInf(in.VerticalErrorCM, 0) || in.VerticalErrorCM < 0 {
		return fail(KindValidation, "invalid_vertical_error", "verticalErrorCm 必须是非负有限数值")
	}
	e := in.Evidence
	if strings.TrimSpace(e.Reference) == "" || strings.TrimSpace(e.Instrument) == "" {
		return fail(KindValidation, "incomplete_evidence", "calibrationEvidence 缺少 reference 或 instrument")
	}
	if e.CalibratedAt.IsZero() {
		return fail(KindValidation, "missing_calibrated_at", "calibratedAt 不能为空")
	}
	if e.CalibratedAt.After(now.Add(5 * time.Minute)) {
		return fail(KindValidation, "future_calibration", "calibratedAt 不能晚于当前时间 5 分钟以上")
	}
	if !hexHashPattern.MatchString(strings.TrimSpace(e.CertificateHash)) {
		return fail(KindValidation, "invalid_certificate_hash", "certificateHash 必须是 64 位十六进制字符串")
	}
	return nil
}

func validateRemediationInput(in RemediationInput) error {
	if err := validateArtifacts(in.Artifacts); err != nil {
		return err
	}
	if in.Resolutions == nil {
		in.Resolutions = map[string]string{}
	}
	if len(in.Resolutions) > 1000 {
		return fail(KindValidation, "too_many_resolutions", "单次整改最多提交 1000 项处置说明")
	}
	for id, note := range in.Resolutions {
		if !identifierPattern.MatchString(id) {
			return fail(KindValidation, "invalid_finding_id", "发现项 ID 格式无效")
		}
		note = strings.TrimSpace(note)
		if note == "" || len([]rune(note)) > 2000 {
			return fail(KindValidation, "invalid_resolution_note", "处置说明不能为空且不能超过 2000 个字符")
		}
	}
	return nil
}

func validateReviewInput(in ReviewInput) error {
	if in.Decision != "approve" && in.Decision != "reject" {
		return fail(KindValidation, "invalid_decision", "decision 必须是 approve 或 reject")
	}
	if in.Decision == "reject" && strings.TrimSpace(in.Reason) == "" {
		return fail(KindValidation, "missing_reason", "退回必须提供理由")
	}
	if len([]rune(in.Reason)) > 2000 {
		return fail(KindValidation, "reason_too_long", "reason 不能超过 2000 个字符")
	}
	return nil
}

func validateVerifyInput(in VerifyInput) error {
	if strings.TrimSpace(in.CredentialNumber) == "" {
		return fail(KindValidation, "missing_credential_number", "credentialNumber 不能为空")
	}
	if !hexHashPattern.MatchString(in.ManifestHash) {
		return fail(KindValidation, "invalid_manifest_hash", "manifestHash 必须是 64 位十六进制字符串")
	}
	if !hexHashPattern.MatchString(in.VerificationCode) {
		return fail(KindValidation, "invalid_verification_code", "verificationCode 必须是 64 位十六进制字符串")
	}
	return nil
}

func validateTimelineQuery(query TimelineQuery) error {
	if query.AfterSequence < 0 {
		return fail(KindValidation, "invalid_after_sequence", "afterSequence 不能为负数")
	}
	if query.Limit < 1 || query.Limit > MaxTimelineLimit {
		return fail(KindValidation, "invalid_timeline_limit", "limit 必须在 1 到 %d 之间", MaxTimelineLimit)
	}
	if query.EventType != "" {
		if _, exists := timelineEventTypes[query.EventType]; !exists {
			return fail(KindValidation, "invalid_event_type", "eventType 不是受支持的业务事件类型")
		}
	}
	return nil
}
