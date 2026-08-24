package domain

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"
)

func TestRevisionContentHashSummaryAndPathRules(t *testing.T) {
	now := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	artifacts := []Artifact{
		{Path: "tiles/readme", SHA256: strings.Repeat("b", 64), SizeBytes: 7},
		{Path: "ortho/A.TIF", SHA256: strings.Repeat("a", 64), SizeBytes: 11, SensitiveTag: true},
	}
	first, err := NewRevision("r1", "c1", "", "engineer", 1, artifacts, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewRevision("r2", "c2", "", "engineer", 1, []Artifact{artifacts[1], artifacts[0]}, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.RevisionContentHash != second.RevisionContentHash {
		t.Fatal("相同成果集合的内容指纹不应受提交顺序影响")
	}
	if first.RegistrationSummary.ArtifactCount != 2 || first.RegistrationSummary.TotalSizeBytes != 18 || first.RegistrationSummary.SensitiveArtifactCount != 1 {
		t.Fatalf("登记摘要错误: %+v", first.RegistrationSummary)
	}
	if first.RegistrationSummary.ByExtension[".tif"].ArtifactCount != 1 || first.RegistrationSummary.ByExtension["[none]"].TotalSizeBytes != 7 {
		t.Fatalf("后缀摘要错误: %+v", first.RegistrationSummary.ByExtension)
	}

	invalidSets := [][]Artifact{
		{{Path: "ortho/A.tif", SHA256: strings.Repeat("a", 64)}, {Path: "ortho/a.tif", SHA256: strings.Repeat("b", 64)}},
		{{Path: "ortho//a.tif", SHA256: strings.Repeat("a", 64)}},
		{{Path: "ortho/./a.tif", SHA256: strings.Repeat("a", 64)}},
		{{Path: "ortho\\a.tif", SHA256: strings.Repeat("a", 64)}},
		{{Path: "ortho/\na.tif", SHA256: strings.Repeat("a", 64)}},
	}
	for index, set := range invalidSets {
		if _, err := NewRevision("bad", "c", "", "engineer", 1, set, now); err == nil {
			t.Fatalf("第 %d 组非法路径未被拒绝", index)
		}
	}
	_, err = NewRevision("overflow", "c", "", "engineer", 1, []Artifact{
		{Path: "a.bin", SHA256: strings.Repeat("a", 64), SizeBytes: math.MaxInt64},
		{Path: "b.bin", SHA256: strings.Repeat("b", 64), SizeBytes: 1},
	}, now)
	if err == nil {
		t.Fatal("大小累计溢出未被拒绝")
	}
}

func TestCalibrationValidityAndCertificateLineage(t *testing.T) {
	now := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	revision, _ := NewRevision("r1", "c1", "", "engineer", 1, []Artifact{{Path: "a.tif", SHA256: strings.Repeat("a", 64), SizeBytes: 1}}, now)
	c, _ := NewCase("c1", "档案", "batch", "crs", "scope", revision, now)
	evidence := CalibrationEvidence{Reference: " CAL-1 ", Instrument: " GNSS ", CalibratedAt: now.AddDate(0, 0, -365), CertificateHash: strings.Repeat("B", 64)}
	if err := c.SubmitCalibration(99, 1, 1, evidence, now); err != nil {
		t.Fatalf("有效期边界上的证据应成功: %v", err)
	}
	stored := c.Revisions[0].CalibrationEvidence
	if !stored.ValidUntil.Equal(now) || stored.ValidationStatus != "valid" || len(stored.EvidenceFingerprint) != 64 {
		t.Fatalf("校准派生状态错误: %+v", stored)
	}

	expiredRevision, _ := NewRevision("r1", "c2", "", "engineer", 1, []Artifact{{Path: "a.tif", SHA256: strings.Repeat("a", 64), SizeBytes: 1}}, now)
	expiredCase, _ := NewCase("c2", "档案", "batch", "crs", "scope", expiredRevision, now)
	expired := evidence
	expired.CalibratedAt = now.AddDate(0, 0, -365).Add(-time.Nanosecond)
	version := expiredCase.Version
	err := expiredCase.SubmitCalibration(99, 1, 1, expired, now)
	var rule *RuleError
	if !errors.As(err, &rule) || rule.Code != "calibration_evidence_expired" || expiredCase.Version != version || expiredCase.Status != StatusDraft {
		t.Fatalf("过期证据处理错误: err=%v case=%+v", err, expiredCase)
	}

	child, _ := NewRevision("r2", "c1", "r1", "engineer", 2, []Artifact{{Path: "a.tif", SHA256: strings.Repeat("b", 64), SizeBytes: 1}}, now)
	child.RevisionDiff = ptrRevisionDiff(CompareRevisions(c.Revisions[0], child))
	c.Revisions = append(c.Revisions, child)
	c.CurrentRevisionID = child.ID
	c.Status = StatusDraft
	conflict := evidence
	conflict.CalibratedAt = stored.CalibratedAt
	conflict.Instrument = "TOTAL-STATION"
	version = c.Version
	err = c.SubmitCalibration(99, 1, 1, conflict, now)
	if !errors.As(err, &rule) || rule.Code != "calibration_evidence_conflict" || c.Version != version || c.Status != StatusDraft {
		t.Fatalf("证书谱系冲突处理错误: %v", err)
	}
}

func TestSensitiveRemediationRequiresContentChange(t *testing.T) {
	now := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	parent, _ := NewRevision("r1", "c1", "", "engineer", 1, []Artifact{{Path: "ortho/raw.tif", SHA256: strings.Repeat("a", 64), SizeBytes: 10, SensitiveTag: true}}, now)
	c, _ := NewCase("c1", "档案", "batch", "crs", "scope", parent, now)
	if err := c.SubmitCalibration(99, 1, 1, CalibrationEvidence{Reference: "CAL", Instrument: "GNSS", CalibratedAt: now, CertificateHash: strings.Repeat("c", 64)}, now); err != nil {
		t.Fatal(err)
	}
	finding, _ := NewFinding("f1", "c1", "r1", "SENSITIVE_ELEMENT", CurrentRuleVersion, SeverityBlocker, "artifact:ortho/raw.tif", "敏感", now)
	applyTestValidation(t, c, "batch-1", []ReviewFinding{finding}, now)
	child, _ := NewRevision("r2", "c1", "r1", "engineer", 2, []Artifact{{Path: "ortho/raw.tif", SHA256: strings.Repeat("a", 64), SizeBytes: 10}}, now)
	version := c.Version
	err := c.AddRemediation(child, map[string]string{"f1": "已处理"}, now)
	var rule *RuleError
	if !errors.As(err, &rule) || rule.Code != "sensitive_artifact_not_remediated" {
		t.Fatalf("仅移除敏感标记但不改变内容摘要应被拒绝: %v", err)
	}
	if c.Version != version || c.CurrentRevisionID != "r1" || c.Status != StatusRemediationRequired || len(c.Revisions) != 1 {
		t.Fatalf("整改拒绝后聚合发生变化: %+v", c)
	}
}

func TestReviewReadinessAndDecisionWarningSnapshot(t *testing.T) {
	now := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	revision, _ := NewRevision("r1", "c1", "", "engineer", 1, []Artifact{{Path: "a.tif", SHA256: strings.Repeat("a", 64), SizeBytes: 1}}, now)
	c, _ := NewCase("c1", "档案", "batch", "crs", "scope", revision, now)
	if err := c.SubmitCalibration(96, 1, 1, CalibrationEvidence{Reference: "CAL", Instrument: "GNSS", CalibratedAt: now, CertificateHash: strings.Repeat("c", 64)}, now); err != nil {
		t.Fatal(err)
	}
	warning, _ := NewFinding("w1", "c1", "r1", "QUALITY_COVERAGE_MARGIN", CurrentRuleVersion, SeverityWarning, "revision", "接近阈值", now)
	applyTestValidation(t, c, "batch-1", []ReviewFinding{warning}, now)
	readiness := c.ReviewReadinessAt(now)
	if !readiness.Ready || readiness.ValidationBatchID != "batch-1" || len(readiness.Warnings) != 1 {
		t.Fatalf("审查就绪度错误: %+v", readiness)
	}
	if err := c.Review("decision-1", true, "同意", "reviewer", readiness, now); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusApproved || len(c.ReviewDecisions) != 1 || c.ReviewDecisions[0].Warnings[0].FindingID != "w1" {
		t.Fatalf("审查决定快照错误: %+v", c.ReviewDecisions)
	}
	if err := c.ValidateIntegrity(); err != nil {
		t.Fatal(err)
	}
}
