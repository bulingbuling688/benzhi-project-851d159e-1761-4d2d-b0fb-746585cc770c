package domain

import (
	"strings"
	"testing"
	"time"
)

func testRevision(t *testing.T, id, caseID, parent string, number int, sensitive bool, now time.Time) DatasetRevision {
	t.Helper()
	hash := "b"
	if sensitive {
		hash = "a"
	}
	r, err := NewRevision(id, caseID, parent, "engineer", number, []Artifact{{Path: "result.tif", SHA256: strings.Repeat(hash, 64), SizeBytes: 20, SensitiveTag: sensitive}}, now)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func applyTestValidation(t *testing.T, c *ReleaseCase, batchID string, findings []ReviewFinding, now time.Time) {
	t.Helper()
	revision, _ := c.CurrentRevision()
	batch, err := BuildValidationBatch(batchID, *revision, findings, c.PreviousValidatedFindings(), now)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.ApplyValidation(batch, findings, now); err != nil {
		t.Fatal(err)
	}
}
func testEvidence(now time.Time) CalibrationEvidence {
	return CalibrationEvidence{Reference: "CAL-1", Instrument: "GNSS", CalibratedAt: now, CertificateHash: strings.Repeat("b", 64)}
}

func TestReleaseWorkflowAndCredential(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	initial := testRevision(t, "rev-1", "case-1", "", 1, true, now)
	c, err := NewCase("case-1", "公开成果", "batch-1", "CGCS2000", "测试范围", initial, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.SubmitCalibration(99, 10, 12, testEvidence(now), now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	finding, err := NewFinding("finding-1", c.ID, c.CurrentRevisionID, "SENSITIVE_ELEMENT", CurrentRuleVersion, SeverityBlocker, "artifact:result.tif", "存在敏感标记", now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	applyTestValidation(t, c, "batch-1", []ReviewFinding{finding}, now.Add(2*time.Minute))
	if c.Status != StatusRemediationRequired {
		t.Fatalf("status=%s", c.Status)
	}
	replacement := testRevision(t, "rev-2", c.ID, "rev-1", 2, false, now.Add(3*time.Minute))
	if err = c.AddRemediation(replacement, map[string]string{"finding-1": "已替换"}, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if c.Revisions[1].RevisionDiff == nil || len(c.Revisions[1].RevisionDiff.ContentChanged) != 1 || len(c.Revisions[1].BlockerResolutionLinks) != 1 {
		t.Fatalf("整改差异或处置关联缺失: %+v", c.Revisions[1])
	}
	if err = c.SubmitCalibration(99, 8, 10, testEvidence(now), now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	applyTestValidation(t, c, "batch-2", nil, now.Add(5*time.Minute))
	latest, _ := c.LatestValidationBatch()
	if latest.SeverityCounts.Blocker != 0 || latest.DifferenceSummary.Resolved.Counts.Blocker != 1 {
		t.Fatalf("复验差异摘要错误: %+v", latest)
	}
	readiness := c.ReviewReadinessAt(now.Add(6 * time.Minute))
	if err = c.Review("decision-1", true, "通过", "reviewer", readiness, now.Add(6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err = c.Freeze("manager", now.Add(7*time.Minute)); err != nil {
		t.Fatal(err)
	}
	secret := []byte("0123456789abcdef0123456789abcdef")
	credential, err := NewCredential("cred-1", "SRC-20260824-000001", c, "manager", now.Add(8*time.Minute), secret)
	if err != nil {
		t.Fatal(err)
	}
	if err = c.Release(credential, now.Add(8*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if c.Status != StatusReleased || !c.Credential.Verify(secret, c.ManifestHash) {
		t.Fatal("凭据未正确签发或验证")
	}
	if err = c.ValidateIntegrity(); err != nil {
		t.Fatalf("完整性检查失败: %v", err)
	}
	if c.Version != 9 {
		t.Fatalf("version=%d", c.Version)
	}
}

func TestReviewRejectionCanCreateNewRevision(t *testing.T) {
	now := time.Now().UTC()
	initial := testRevision(t, "r1", "c1", "", 1, false, now)
	c, _ := NewCase("c1", "title", "batch", "crs", "scope", initial, now)
	_ = c.SubmitCalibration(99, 1, 1, testEvidence(now), now)
	applyTestValidation(t, c, "batch-1", nil, now)
	readiness := c.ReviewReadinessAt(now)
	if err := c.Review("decision-1", false, "边界仍需裁切", "reviewer", readiness, now); err != nil {
		t.Fatal(err)
	}
	replacement := testRevision(t, "r2", "c1", "r1", 2, false, now)
	if err := c.AddRemediation(replacement, map[string]string{}, now); err != nil {
		t.Fatalf("人工退回后应允许整改: %v", err)
	}
	if c.Status != StatusDraft || c.ReviewNote != "" {
		t.Fatalf("整改后状态异常: %s", c.Status)
	}
}

func TestIllegalTransitionsAndArtifacts(t *testing.T) {
	now := time.Now().UTC()
	initial := testRevision(t, "r1", "c1", "", 1, false, now)
	c, _ := NewCase("c1", "title", "batch", "crs", "scope", initial, now)
	if err := c.Review("decision-1", true, "", "reviewer", ReviewReadiness{}, now); err == nil {
		t.Fatal("draft 不应允许审查")
	}
	if _, err := NewRevision("r2", "c1", "r1", "actor", 2, []Artifact{{Path: "x", SHA256: "short", SizeBytes: 1}}, now); err == nil {
		t.Fatal("无效摘要应被拒绝")
	}
	if err := c.SubmitCalibration(101, 1, 1, testEvidence(now), now); err == nil {
		t.Fatal("无效覆盖率应被拒绝")
	}
	c.Status = Status("unknown")
	if err := c.ValidateIntegrity(); err == nil {
		t.Fatal("未知状态应被完整性检查拒绝")
	}
}
