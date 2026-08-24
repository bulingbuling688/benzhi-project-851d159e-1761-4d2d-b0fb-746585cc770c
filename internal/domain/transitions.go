package domain

import "time"

func (c *ReleaseCase) SubmitCalibration(coverage, horizontal, vertical float64, evidence CalibrationEvidence, now time.Time) error {
	if c.Status != StatusDraft {
		return invalid("invalid_state", "当前状态 %s 不能提交校准证据", c.Status)
	}
	r, ok := c.CurrentRevision()
	if !ok {
		return invalid("missing_revision", "当前候选修订不存在")
	}
	prepared, err := prepareCalibrationEvidence(evidence, now)
	if err != nil {
		return err
	}
	for _, revision := range c.Revisions {
		registered := revision.CalibrationEvidence
		if registered == nil || registered.CertificateHash != prepared.CertificateHash {
			continue
		}
		if !sameCertificateIdentity(*registered, prepared) {
			return invalid("calibration_evidence_conflict", "相同 certificateHash 在修订谱系中的 reference、instrument 或 calibratedAt 不一致")
		}
	}
	if err := r.SetCalibration(coverage, horizontal, vertical, prepared, now); err != nil {
		return err
	}
	c.Status = StatusValidating
	c.Touch(now)
	return nil
}

func (c *ReleaseCase) Freeze(actor string, now time.Time) error {
	m, hash, err := BuildManifest(c, actor, now)
	if err != nil {
		return err
	}
	c.Manifest = &m
	c.ManifestHash = hash
	c.Status = StatusFrozen
	c.Touch(now)
	return nil
}

func (c *ReleaseCase) Release(cred ReleaseCredential, now time.Time) error {
	if c.Status != StatusFrozen || cred.CaseID != c.ID || cred.RevisionID != c.CurrentRevisionID || cred.ManifestHash != c.ManifestHash {
		return invalid("invalid_credential", "凭据与冻结档案不一致")
	}
	c.Credential = &cred
	c.Status = StatusReleased
	c.Touch(now)
	return nil
}
