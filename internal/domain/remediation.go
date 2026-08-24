package domain

import (
	"sort"
	"strings"
	"time"
)

type RevisionDiff struct {
	Added                []string `json:"added"`
	Removed              []string `json:"removed"`
	ContentChanged       []string `json:"contentChanged"`
	MetadataChanged      []string `json:"metadataChanged"`
	Unchanged            []string `json:"unchanged"`
	TotalSizeChangeBytes int64    `json:"totalSizeChangeBytes"`
}

type BlockerResolutionLink struct {
	FindingID            string   `json:"findingId"`
	RuleCode             string   `json:"ruleCode"`
	DifferencePaths      []string `json:"differencePaths"`
	ResolutionNote       string   `json:"resolutionNote"`
	ResolvedByRevisionID string   `json:"resolvedByRevisionId"`
	Status               string   `json:"status"`
}

func CompareRevisions(parent, next DatasetRevision) RevisionDiff {
	parentByPath := make(map[string]Artifact, len(parent.Artifacts))
	nextByPath := make(map[string]Artifact, len(next.Artifacts))
	for _, item := range parent.Artifacts {
		parentByPath[item.Path] = item
	}
	for _, item := range next.Artifacts {
		nextByPath[item.Path] = item
	}
	diff := RevisionDiff{TotalSizeChangeBytes: next.RegistrationSummary.TotalSizeBytes - parent.RegistrationSummary.TotalSizeBytes}
	for path, before := range parentByPath {
		after, exists := nextByPath[path]
		if !exists {
			diff.Removed = append(diff.Removed, path)
			continue
		}
		switch {
		case before.SHA256 != after.SHA256:
			diff.ContentChanged = append(diff.ContentChanged, path)
		case before.SizeBytes != after.SizeBytes || before.SensitiveTag != after.SensitiveTag:
			diff.MetadataChanged = append(diff.MetadataChanged, path)
		default:
			diff.Unchanged = append(diff.Unchanged, path)
		}
	}
	for path := range nextByPath {
		if _, exists := parentByPath[path]; !exists {
			diff.Added = append(diff.Added, path)
		}
	}
	sort.Strings(diff.Added)
	sort.Strings(diff.Removed)
	sort.Strings(diff.ContentChanged)
	sort.Strings(diff.MetadataChanged)
	sort.Strings(diff.Unchanged)
	return diff
}

func artifactLocationPath(location string) (string, bool) {
	const prefix = "artifact:"
	if !strings.HasPrefix(location, prefix) {
		return "", false
	}
	value, err := normalizeArtifactPath(strings.TrimPrefix(location, prefix))
	return value, err == nil
}

func containsPath(paths []string, target string) bool {
	index := sort.SearchStrings(paths, target)
	return index < len(paths) && paths[index] == target
}

func buildResolutionLinks(parent, next DatasetRevision, blockers []ReviewFinding, resolutions map[string]string) ([]BlockerResolutionLink, error) {
	diff := CompareRevisions(parent, next)
	changedPaths := make([]string, 0, len(diff.Added)+len(diff.Removed)+len(diff.ContentChanged)+len(diff.MetadataChanged))
	changedPaths = append(changedPaths, diff.Added...)
	changedPaths = append(changedPaths, diff.Removed...)
	changedPaths = append(changedPaths, diff.ContentChanged...)
	changedPaths = append(changedPaths, diff.MetadataChanged...)
	sort.Strings(changedPaths)
	nextArtifacts := make(map[string]Artifact, len(next.Artifacts))
	for _, artifact := range next.Artifacts {
		nextArtifacts[artifact.Path] = artifact
	}
	links := make([]BlockerResolutionLink, 0, len(blockers))
	for _, blocker := range blockers {
		note := strings.TrimSpace(resolutions[blocker.ID])
		if note == "" {
			return nil, invalid("missing_resolution", "阻断发现项 %s 缺少处置说明", blocker.ID)
		}
		link := BlockerResolutionLink{FindingID: blocker.ID, RuleCode: blocker.RuleCode, DifferencePaths: append([]string{}, changedPaths...), ResolutionNote: note, ResolvedByRevisionID: next.ID, Status: "pending_revalidation"}
		if artifactPath, specific := artifactLocationPath(blocker.LocationRef); specific && blocker.RuleCode == "SENSITIVE_ELEMENT" {
			removed := containsPath(diff.Removed, artifactPath)
			changed := containsPath(diff.ContentChanged, artifactPath)
			artifact, retained := nextArtifacts[artifactPath]
			if !removed && (!changed || !retained || artifact.SensitiveTag) {
				return nil, invalid("sensitive_artifact_not_remediated", "敏感成果 %s 必须删除，或以新摘要且不带敏感标记的成果替换", artifactPath)
			}
			link.DifferencePaths = []string{artifactPath}
			link.Status = "resolved_by_file_change"
		}
		links = append(links, link)
	}
	sort.Slice(links, func(i, j int) bool { return links[i].FindingID < links[j].FindingID })
	return links, nil
}

func (c *ReleaseCase) AddRemediation(revision DatasetRevision, resolutions map[string]string, now time.Time) error {
	if c.Status != StatusRemediationRequired {
		return invalid("invalid_state", "当前状态 %s 不能提交整改修订", c.Status)
	}
	parent, exists := c.CurrentRevision()
	if !exists || revision.CaseID != c.ID || revision.ParentRevisionID != c.CurrentRevisionID || revision.RevisionNumber != len(c.Revisions)+1 {
		return invalid("invalid_revision_lineage", "整改修订必须直接继承当前修订")
	}
	open := c.OpenBlockers()
	if len(open) == 0 && c.ReviewNote == "" {
		return invalid("no_blockers", "没有需要处置的阻断发现项")
	}
	links, err := buildResolutionLinks(*parent, revision, open, resolutions)
	if err != nil {
		return err
	}
	revision.RevisionDiff = ptrRevisionDiff(CompareRevisions(*parent, revision))
	revision.BlockerResolutionLinks = links
	for i := range c.Findings {
		if c.Findings[i].Status != FindingOpen || c.Findings[i].RevisionID != parent.ID {
			continue
		}
		for _, link := range links {
			if link.FindingID == c.Findings[i].ID && link.Status == "resolved_by_file_change" {
				if err := c.Findings[i].Resolve(link.ResolutionNote, revision.ID, now); err != nil {
					return err
				}
			}
		}
	}
	c.Revisions = append(c.Revisions, revision)
	c.CurrentRevisionID = revision.ID
	c.Status = StatusDraft
	c.ReviewNote = ""
	c.Touch(now)
	return nil
}

func ptrRevisionDiff(value RevisionDiff) *RevisionDiff { return &value }
