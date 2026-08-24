package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"path"
	"sort"
	"strings"
	"unicode"
)

type ArtifactGroupSummary struct {
	ArtifactCount          int   `json:"artifactCount"`
	TotalSizeBytes         int64 `json:"totalSizeBytes"`
	SensitiveArtifactCount int   `json:"sensitiveArtifactCount"`
}

type ArtifactRegistrationSummary struct {
	ArtifactCount          int                             `json:"artifactCount"`
	TotalSizeBytes         int64                           `json:"totalSizeBytes"`
	SensitiveArtifactCount int                             `json:"sensitiveArtifactCount"`
	ByExtension            map[string]ArtifactGroupSummary `json:"byExtension"`
}

type artifactHashItem struct {
	Path         string `json:"path"`
	SHA256       string `json:"sha256"`
	SizeBytes    int64  `json:"sizeBytes"`
	SensitiveTag bool   `json:"sensitiveTag"`
}

func normalizeArtifactPath(raw string) (string, error) {
	for _, r := range raw {
		if unicode.IsControl(r) {
			return "", invalid("invalid_artifact_path", "成果文件 path 不能包含控制字符")
		}
	}
	value := strings.TrimSpace(raw)
	if value == "" || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return "", invalid("invalid_artifact_path", "成果文件 path 必须是非空相对逻辑路径")
	}
	if strings.Contains(value, "\\") || strings.Contains(value, "//") {
		return "", invalid("invalid_artifact_path", "成果文件 path 不能包含反斜杠或重复分隔符")
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "." || segment == ".." || segment == "" {
			return "", invalid("invalid_artifact_path", "成果文件 path 不能包含点号目录或空目录")
		}
	}
	return value, nil
}

func prepareArtifacts(artifacts []Artifact) ([]Artifact, string, ArtifactRegistrationSummary, error) {
	if len(artifacts) == 0 {
		return nil, "", ArtifactRegistrationSummary{}, invalid("invalid_revision", "修订至少登记一个成果文件")
	}
	items := append([]Artifact(nil), artifacts...)
	seen := make(map[string]string, len(items))
	for i := range items {
		normalized, err := normalizeArtifactPath(items[i].Path)
		if err != nil {
			return nil, "", ArtifactRegistrationSummary{}, err
		}
		if len([]rune(normalized)) > 1024 {
			return nil, "", ArtifactRegistrationSummary{}, invalid("invalid_artifact_path", "成果文件 path 不能超过 1024 个字符")
		}
		items[i].Path = normalized
		items[i].SHA256 = strings.ToLower(strings.TrimSpace(items[i].SHA256))
		if !sha256Pattern.MatchString(items[i].SHA256) || items[i].SizeBytes < 0 {
			return nil, "", ArtifactRegistrationSummary{}, invalid("invalid_artifact", "成果文件 sha256 或 sizeBytes 无效")
		}
		folded := strings.ToLower(normalized)
		if previous, exists := seen[folded]; exists {
			if previous == normalized {
				return nil, "", ArtifactRegistrationSummary{}, invalid("duplicate_artifact", "成果文件路径重复: %s", normalized)
			}
			return nil, "", ArtifactRegistrationSummary{}, invalid("artifact_path_case_conflict", "成果文件路径仅大小写不同: %s 与 %s", previous, normalized)
		}
		seen[folded] = normalized
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })

	summary := ArtifactRegistrationSummary{ArtifactCount: len(items), ByExtension: map[string]ArtifactGroupSummary{}}
	hashItems := make([]artifactHashItem, 0, len(items))
	for _, item := range items {
		if item.SizeBytes > math.MaxInt64-summary.TotalSizeBytes {
			return nil, "", ArtifactRegistrationSummary{}, invalid("artifact_size_overflow", "成果文件大小累计溢出")
		}
		summary.TotalSizeBytes += item.SizeBytes
		if item.SensitiveTag {
			summary.SensitiveArtifactCount++
		}
		extension := strings.ToLower(path.Ext(item.Path))
		if extension == "" {
			extension = "[none]"
		}
		group := summary.ByExtension[extension]
		if item.SizeBytes > math.MaxInt64-group.TotalSizeBytes {
			return nil, "", ArtifactRegistrationSummary{}, invalid("artifact_size_overflow", "成果文件后缀分组大小累计溢出")
		}
		group.ArtifactCount++
		group.TotalSizeBytes += item.SizeBytes
		if item.SensitiveTag {
			group.SensitiveArtifactCount++
		}
		summary.ByExtension[extension] = group
		hashItems = append(hashItems, artifactHashItem{Path: item.Path, SHA256: item.SHA256, SizeBytes: item.SizeBytes, SensitiveTag: item.SensitiveTag})
	}
	material, err := json.Marshal(hashItems)
	if err != nil {
		return nil, "", ArtifactRegistrationSummary{}, err
	}
	digest := sha256.Sum256(material)
	return items, hex.EncodeToString(digest[:]), summary, nil
}
