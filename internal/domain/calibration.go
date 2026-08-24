package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

const CalibrationValidityDays = 365

type calibrationFingerprintMaterial struct {
	CertificateHash string    `json:"certificateHash"`
	Reference       string    `json:"reference"`
	Instrument      string    `json:"instrument"`
	CalibratedAt    time.Time `json:"calibratedAt"`
}

func prepareCalibrationEvidence(evidence CalibrationEvidence, now time.Time) (CalibrationEvidence, error) {
	evidence.Reference = strings.TrimSpace(evidence.Reference)
	evidence.Instrument = strings.TrimSpace(evidence.Instrument)
	evidence.CertificateHash = strings.ToLower(strings.TrimSpace(evidence.CertificateHash))
	evidence.CalibratedAt = evidence.CalibratedAt.UTC()
	if evidence.Reference == "" || evidence.Instrument == "" || !sha256Pattern.MatchString(evidence.CertificateHash) || evidence.CalibratedAt.IsZero() {
		return CalibrationEvidence{}, invalid("invalid_evidence", "校准证据缺少 reference、instrument、calibratedAt 或有效 certificateHash")
	}
	evidence.ValidUntil = evidence.CalibratedAt.AddDate(0, 0, CalibrationValidityDays)
	if now.UTC().After(evidence.ValidUntil) {
		return CalibrationEvidence{}, invalid("calibration_evidence_expired", "校准证据已于 %s 过期", evidence.ValidUntil.Format(time.RFC3339))
	}
	material, err := json.Marshal(calibrationFingerprintMaterial{CertificateHash: evidence.CertificateHash, Reference: evidence.Reference, Instrument: evidence.Instrument, CalibratedAt: evidence.CalibratedAt})
	if err != nil {
		return CalibrationEvidence{}, err
	}
	sum := sha256.Sum256(material)
	evidence.EvidenceFingerprint = hex.EncodeToString(sum[:])
	evidence.ValidationStatus = "valid"
	return evidence, nil
}

func sameCertificateIdentity(left, right CalibrationEvidence) bool {
	return left.Reference == right.Reference && left.Instrument == right.Instrument && left.CalibratedAt.Equal(right.CalibratedAt)
}

func validateCalibrationIntegrity(evidence CalibrationEvidence) error {
	rebuilt, err := prepareCalibrationEvidence(evidence, evidence.ValidUntil)
	if err != nil {
		return err
	}
	if !evidence.ValidUntil.Equal(rebuilt.ValidUntil) || evidence.ValidationStatus != rebuilt.ValidationStatus || evidence.EvidenceFingerprint != rebuilt.EvidenceFingerprint {
		return invalid("calibration_summary_mismatch", "校准证据的有效期、校验状态或指纹与原始证据不一致")
	}
	return nil
}
