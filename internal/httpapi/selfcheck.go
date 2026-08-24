package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"surveyrelease/internal/application"
	"surveyrelease/internal/domain"
)

type selfcheckClient struct {
	base    string
	client  *http.Client
	counter int
}

func RunSelfcheck(ctx context.Context, address string) error {
	host := address
	if strings.HasPrefix(host, "[::]") {
		host = "127.0.0.1" + strings.TrimPrefix(host, "[::]")
	}
	if strings.HasPrefix(host, "0.0.0.0:") {
		host = "127.0.0.1:" + strings.TrimPrefix(host, "0.0.0.0:")
	}
	s := &selfcheckClient{base: "http://" + host, client: &http.Client{}}
	shaA := strings.Repeat("a", 64)
	shaB := strings.Repeat("b", 64)
	cert := strings.Repeat("c", 64)
	create := application.CreateCaseInput{Title: "自检航测成果", AcquisitionBatch: "SELF-CHECK-001", CoordinateReference: "CGCS2000 / 3-degree Gauss-Kruger", ScopeDescription: "自检范围", Artifacts: []domain.Artifact{{Path: "ortho/initial.tif", SHA256: shaA, SizeBytes: 1024, SensitiveTag: true}}}
	var created application.CaseResponse
	if err := s.write(ctx, "POST", "/api/v1/release-cases", application.RoleEngineer, "", create, &created); err != nil {
		return err
	}
	caseID := created.Case.ID
	version := created.Version
	if created.Case.Revisions[0].RevisionContentHash == "" || created.Case.Revisions[0].RegistrationSummary.ArtifactCount != 1 {
		return fmt.Errorf("自检首修订内容指纹或登记摘要缺失")
	}
	evidence := application.CalibrationInput{CoveragePercent: 99, HorizontalErrorCM: 10, VerticalErrorCM: 15, Evidence: application.CalibrationEvidenceInput{Reference: "CAL-SELF-001", Instrument: "GNSS-SELF", CalibratedAt: created.Case.CreatedAt, CertificateHash: cert}}
	var state application.CaseResponse
	if err := s.write(ctx, "POST", fmt.Sprintf("/api/v1/release-cases/%s/calibration", caseID), application.RoleEngineer, formatVersion(version), evidence, &state); err != nil {
		return err
	}
	version = state.Version
	if err := s.write(ctx, "POST", fmt.Sprintf("/api/v1/release-cases/%s/validation", caseID), application.RoleReviewer, formatVersion(version), nil, &state); err != nil {
		return err
	}
	version = state.Version
	if state.Case.Status != domain.StatusRemediationRequired {
		return fmt.Errorf("自检期望 remediation_required，实际 %s", state.Case.Status)
	}
	firstBatch, ok := state.Case.LatestValidationBatch()
	if !ok || firstBatch.ValidationFingerprint == "" || firstBatch.SeverityCounts.Blocker != 1 {
		return fmt.Errorf("自检首个核验批次摘要无效")
	}
	resolutions := map[string]string{}
	for _, f := range state.Case.OpenBlockers() {
		resolutions[f.ID] = "已用脱敏成果替换"
	}
	remediation := application.RemediationInput{Artifacts: []domain.Artifact{{Path: "ortho/public.tif", SHA256: shaB, SizeBytes: 1000}}, Resolutions: resolutions}
	if err := s.write(ctx, "POST", fmt.Sprintf("/api/v1/release-cases/%s/remediations", caseID), application.RoleEngineer, formatVersion(version), remediation, &state); err != nil {
		return err
	}
	version = state.Version
	current, ok := state.Case.CurrentRevision()
	if !ok || current.RevisionDiff == nil || len(current.RevisionDiff.Removed) != 1 || len(current.BlockerResolutionLinks) != 1 {
		return fmt.Errorf("自检整改差异或处置关联无效")
	}
	if err := s.write(ctx, "POST", fmt.Sprintf("/api/v1/release-cases/%s/calibration", caseID), application.RoleEngineer, formatVersion(version), evidence, &state); err != nil {
		return err
	}
	version = state.Version
	if err := s.write(ctx, "POST", fmt.Sprintf("/api/v1/release-cases/%s/validation", caseID), application.RoleReviewer, formatVersion(version), nil, &state); err != nil {
		return err
	}
	version = state.Version
	if state.Case.Status != domain.StatusReviewing {
		return fmt.Errorf("自检期望 reviewing，实际 %s", state.Case.Status)
	}
	secondBatch, ok := state.Case.LatestValidationBatch()
	if !ok || secondBatch.DifferenceSummary.Resolved.Counts.Blocker != 1 || state.ReviewReadiness == nil || !state.ReviewReadiness.Ready {
		return fmt.Errorf("自检复验差异或审查就绪度无效")
	}
	if err := s.write(ctx, "POST", fmt.Sprintf("/api/v1/release-cases/%s/review", caseID), application.RoleReviewer, formatVersion(version), application.ReviewInput{Decision: "approve", Reason: "自检批准"}, &state); err != nil {
		return err
	}
	version = state.Version
	if err := s.write(ctx, "POST", fmt.Sprintf("/api/v1/release-cases/%s/freeze", caseID), application.RoleReleaseManager, formatVersion(version), nil, &state); err != nil {
		return err
	}
	version = state.Version
	var issued application.CredentialResponse
	if err := s.write(ctx, "POST", fmt.Sprintf("/api/v1/release-cases/%s/credentials", caseID), application.RoleReleaseManager, formatVersion(version), nil, &issued); err != nil {
		return err
	}
	var detail application.CaseResponse
	if err := s.read(ctx, fmt.Sprintf("/api/v1/release-cases/%s", caseID), &detail); err != nil {
		return err
	}
	if detail.Case.Status != domain.StatusReleased {
		return fmt.Errorf("自检期望 released，实际 %s", detail.Case.Status)
	}
	after := int64(0)
	sequences := make([]int64, 0, 9)
	for pageNumber := 0; pageNumber < 3; pageNumber++ {
		var page application.TimelinePage
		path := fmt.Sprintf("/api/v1/release-cases/%s/timeline?afterSequence=%d&limit=3", caseID, after)
		if err := s.read(ctx, path, &page); err != nil {
			return err
		}
		if page.ReturnedCount != 3 || page.LedgerHeadSequence < 9 || page.LedgerHeadHash == "" {
			return fmt.Errorf("自检时间线第 %d 页元数据无效", pageNumber+1)
		}
		for _, event := range page.Events {
			if !event.HashVerified || (len(sequences) > 0 && event.Sequence <= sequences[len(sequences)-1]) {
				return fmt.Errorf("自检时间线哈希标记或序号无效")
			}
			sequences = append(sequences, event.Sequence)
		}
		after = page.NextAfterSequence
		if pageNumber < 2 && !page.HasMore {
			return fmt.Errorf("自检时间线第 %d 页过早结束", pageNumber+1)
		}
		if pageNumber == 2 && page.HasMore {
			return fmt.Errorf("自检时间线末页仍标记 hasMore")
		}
	}
	if len(sequences) != 9 {
		return fmt.Errorf("自检期望 9 条审计事件，实际 %d", len(sequences))
	}
	var filtered application.TimelinePage
	if err := s.read(ctx, fmt.Sprintf("/api/v1/release-cases/%s/timeline?eventType=credential.issued", caseID), &filtered); err != nil {
		return err
	}
	if filtered.ReturnedCount != 1 || filtered.Events[0].EventType != "credential.issued" {
		return fmt.Errorf("自检签发事件筛选无效")
	}
	verify := application.VerifyInput{CredentialNumber: issued.Credential.CredentialNumber, VerificationCode: issued.Credential.VerificationCode, ManifestHash: issued.Credential.ManifestHash}
	var result application.VerificationResult
	if err := s.write(ctx, "POST", fmt.Sprintf("/api/v1/release-cases/%s/credential-verification", caseID), "", "", verify, &result); err != nil {
		return err
	}
	if !result.Valid {
		return fmt.Errorf("自检凭据验证失败: %s", result.Reason)
	}
	return nil
}

func (s *selfcheckClient) write(ctx context.Context, method, path string, role application.ActorRole, version string, body any, out any) error {
	s.counter++
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if role != "" {
		req.Header.Set("Actor-Role", string(role))
		req.Header.Set("Actor-Name", "selfcheck")
	}
	if method != "GET" && role != "" {
		req.Header.Set("Idempotency-Key", fmt.Sprintf("selfcheck-%02d", s.counter))
	}
	if version != "" {
		req.Header.Set("If-Match", version)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
		return fmt.Errorf("%s %s 返回 %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
func (s *selfcheckClient) read(ctx context.Context, path string, out any) error {
	return s.write(ctx, "GET", path, "", "", nil, out)
}
