package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

type ReleaseCredential struct {
	ID               string    `json:"id"`
	CaseID           string    `json:"caseId"`
	CredentialNumber string    `json:"credentialNumber"`
	RevisionID       string    `json:"revisionId"`
	ManifestHash     string    `json:"manifestHash"`
	VerificationCode string    `json:"verificationCode"`
	IssuedBy         string    `json:"issuedBy"`
	IssuedAt         time.Time `json:"issuedAt"`
}

func CredentialCode(secret []byte, number, caseID, revisionID, manifestHash string, issuedAt time.Time) string {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(strings.Join([]string{number, caseID, revisionID, manifestHash, issuedAt.UTC().Format(time.RFC3339Nano)}, "\n")))
	return hex.EncodeToString(h.Sum(nil))
}

func NewCredential(id, number string, c *ReleaseCase, actor string, now time.Time, secret []byte) (ReleaseCredential, error) {
	if c.Status != StatusFrozen || c.Manifest == nil || c.ManifestHash == "" {
		return ReleaseCredential{}, invalid("invalid_state", "只有 frozen 档案可以签发凭据")
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(number) == "" || len(secret) < 16 {
		return ReleaseCredential{}, invalid("invalid_credential", "凭据标识、编号或签名密钥无效")
	}
	now = now.UTC()
	cred := ReleaseCredential{ID: id, CaseID: c.ID, CredentialNumber: number, RevisionID: c.CurrentRevisionID, ManifestHash: c.ManifestHash, IssuedBy: actor, IssuedAt: now}
	cred.VerificationCode = CredentialCode(secret, number, c.ID, c.CurrentRevisionID, c.ManifestHash, now)
	return cred, nil
}

func (c ReleaseCredential) Verify(secret []byte, manifestHash string) bool {
	if !hmac.Equal([]byte(c.ManifestHash), []byte(manifestHash)) {
		return false
	}
	expected := CredentialCode(secret, c.CredentialNumber, c.CaseID, c.RevisionID, c.ManifestHash, c.IssuedAt)
	return hmac.Equal([]byte(expected), []byte(c.VerificationCode))
}
