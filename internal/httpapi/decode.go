package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"surveyrelease/internal/application"
)

const maxRequestBody = 1 << 20

func decodeStrict(w http.ResponseWriter, r *http.Request, dst any) error {
	if media := r.Header.Get("Content-Type"); media != "" && !strings.HasPrefix(strings.ToLower(media), "application/json") {
		return applicationError("unsupported_media_type", "Content-Type 必须是 application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if err == io.EOF {
			return applicationError("empty_body", "JSON 请求体不能为空")
		}
		return applicationError("invalid_json", fmt.Sprintf("JSON 请求体无效: %v", err))
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return applicationError("multiple_json_values", "请求体只能包含一个 JSON 值")
	}
	return nil
}

func applicationError(code, message string) error {
	return &application.AppError{Kind: application.KindValidation, Code: code, Message: message}
}

func parseMeta(r *http.Request, expected bool) (application.CommandMeta, error) {
	meta := application.CommandMeta{Actor: application.Actor{Name: strings.TrimSpace(r.Header.Get("Actor-Name")), Role: application.ActorRole(strings.TrimSpace(r.Header.Get("Actor-Role")))}, IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key"))}
	if raw := strings.TrimSpace(r.Header.Get("If-Match")); raw != "" {
		raw = strings.Trim(raw, "\"")
		v, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || v < 0 {
			return meta, applicationError("invalid_expected_version", "If-Match 必须是非负整数版本")
		}
		meta.ExpectedVersion = &v
	} else if expected {
		return meta, applicationError("missing_expected_version", "If-Match 不能为空")
	}
	return meta, nil
}

func formatVersion(v int64) string { return fmt.Sprintf("\"%d\"", v) }
