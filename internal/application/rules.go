package application

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"surveyrelease/internal/domain"
	"time"
)

const RuleVersion = domain.CurrentRuleVersion

type FindingSpec struct {
	Code     string
	Severity domain.Severity
	Location string
	Message  string
}

func EvaluateRevision(c *domain.ReleaseCase, now time.Time) ([]domain.ReviewFinding, error) {
	r, ok := c.CurrentRevision()
	if !ok || !r.HasCompleteCalibration() {
		return nil, fail(KindState, "incomplete_calibration", "当前修订缺少完整校准测量值")
	}
	specs := []FindingSpec{}
	if *r.CoveragePercent < 95 {
		specs = append(specs, FindingSpec{"QUALITY_COVERAGE", domain.SeverityBlocker, "revision", fmt.Sprintf("覆盖率 %.2f%% 低于 95.00%%", *r.CoveragePercent)})
	}
	if *r.HorizontalErrorCM > 20 {
		specs = append(specs, FindingSpec{"QUALITY_HORIZONTAL", domain.SeverityBlocker, "revision", fmt.Sprintf("平面误差 %.2fcm 超过 20.00cm", *r.HorizontalErrorCM)})
	}
	if *r.VerticalErrorCM > 30 {
		specs = append(specs, FindingSpec{"QUALITY_VERTICAL", domain.SeverityBlocker, "revision", fmt.Sprintf("高程误差 %.2fcm 超过 30.00cm", *r.VerticalErrorCM)})
	}
	for _, a := range r.Artifacts {
		if a.SensitiveTag {
			specs = append(specs, FindingSpec{"SENSITIVE_ELEMENT", domain.SeverityBlocker, "artifact:" + a.Path, "成果文件标记了未脱敏要素"})
		}
	}
	if *r.CoveragePercent < 98 && *r.CoveragePercent >= 95 {
		specs = append(specs, FindingSpec{"QUALITY_COVERAGE_MARGIN", domain.SeverityWarning, "revision", "覆盖率已通过但接近规则阈值"})
	}
	sort.Slice(specs, func(i, j int) bool {
		if specs[i].Code == specs[j].Code {
			return specs[i].Location < specs[j].Location
		}
		return specs[i].Code < specs[j].Code
	})
	findings := make([]domain.ReviewFinding, 0, len(specs))
	for _, spec := range specs {
		h := sha256.Sum256([]byte(c.ID + "\n" + r.ID + "\n" + spec.Code + "\n" + spec.Location))
		f, err := domain.NewFinding("fdg_"+hex.EncodeToString(h[:8]), c.ID, r.ID, spec.Code, RuleVersion, spec.Severity, spec.Location, spec.Message, now)
		if err != nil {
			return nil, err
		}
		findings = append(findings, f)
	}
	return findings, nil
}
