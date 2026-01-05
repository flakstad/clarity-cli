package tui

import (
        "regexp"
        "strings"
)

type linkNavTargetKind int

const (
        linkNavTargetURL linkNavTargetKind = iota
        linkNavTargetAttachment
)

type linkNavTarget struct {
        Kind   linkNavTargetKind
        Target string
        Label  string
}

var (
        // Basic markdown link target extraction. We keep it intentionally permissive; trimming cleans up.
        reMarkdownLink = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
        // Basic URL detection for plain text.
        reURL = regexp.MustCompile(`(?i)\bhttps?://[^\s<>()]+`)
        // Attachment ids are stable and easy to recognize.
        reAttachmentID = regexp.MustCompile(`\batt-[a-z0-9]+\b`)
)

func extractLinkTargets(md string) []linkNavTarget {
        md = strings.TrimSpace(md)
        if md == "" {
                return nil
        }

        seen := map[string]bool{}
        out := make([]linkNavTarget, 0, 8)

        add := func(kind linkNavTargetKind, target, label string) {
                target = strings.TrimSpace(target)
                if target == "" {
                        return
                }
                key := strings.TrimSpace(strings.ToLower(kind.String() + ":" + target))
                if key == "" || seen[key] {
                        return
                }
                seen[key] = true
                out = append(out, linkNavTarget{Kind: kind, Target: target, Label: strings.TrimSpace(label)})
        }

        for _, m := range reMarkdownLink.FindAllStringSubmatch(md, -1) {
                if len(m) < 2 {
                        continue
                }
                target := strings.TrimSpace(m[1])
                if target == "" {
                        continue
                }
                if reAttachmentID.MatchString(target) {
                        add(linkNavTargetAttachment, reAttachmentID.FindString(target), target)
                        continue
                }
                if reURL.MatchString(target) {
                        add(linkNavTargetURL, reURL.FindString(target), target)
                        continue
                }
                // Accept non-http links (e.g. file://) as URLs for OS open.
                add(linkNavTargetURL, target, target)
        }

        for _, u := range reURL.FindAllString(md, -1) {
                add(linkNavTargetURL, u, u)
        }
        for _, a := range reAttachmentID.FindAllString(md, -1) {
                add(linkNavTargetAttachment, a, a)
        }

        return out
}

func extractURLTargets(md string) []string {
        md = strings.TrimSpace(md)
        if md == "" {
                return nil
        }
        seen := map[string]bool{}
        out := make([]string, 0, 8)
        for _, m := range reMarkdownLink.FindAllStringSubmatch(md, -1) {
                if len(m) < 2 {
                        continue
                }
                target := strings.TrimSpace(m[1])
                if target == "" {
                        continue
                }
                // Accept both http(s) and other schemes (e.g. file://) as OS-openable.
                if reAttachmentID.MatchString(target) {
                        continue
                }
                key := strings.ToLower(target)
                if seen[key] {
                        continue
                }
                seen[key] = true
                out = append(out, target)
        }
        for _, u := range reURL.FindAllString(md, -1) {
                key := strings.ToLower(strings.TrimSpace(u))
                if key == "" || seen[key] {
                        continue
                }
                seen[key] = true
                out = append(out, strings.TrimSpace(u))
        }
        return out
}

func (k linkNavTargetKind) String() string {
        switch k {
        case linkNavTargetAttachment:
                return "attachment"
        default:
                return "url"
        }
}
