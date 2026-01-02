package gitrepo

import (
        "context"
        "encoding/json"
        "fmt"
        "sort"
        "strings"
)

type stagedEvent struct {
        Type     string          `json:"type"`
        EntityID string          `json:"entityId"`
        Payload  json.RawMessage `json:"payload"`
}

// StagedEventSummary inspects staged changes for `events/*.jsonl` and summarizes newly-added events.
// Best-effort: failures return empty summary.
func StagedEventSummary(ctx context.Context, workspaceDir string, maxEvents int) (summary string, counts map[string]int, err error) {
        st, err := GetStatus(ctx, workspaceDir)
        if err != nil {
                return "", nil, err
        }
        if !st.IsRepo {
                return "", map[string]int{}, nil
        }
        if maxEvents <= 0 {
                maxEvents = 25
        }

        // We only care about added JSONL lines (new events) in staged diff.
        diff, err := runGit(ctx, st.Root, "diff", "--cached", "--unified=0", "--no-color", "--", "events/*.jsonl")
        if err != nil {
                return "", nil, err
        }

        events := parseAddedJSONLines(diff)
        if len(events) == 0 {
                return "", map[string]int{}, nil
        }

        counts = map[string]int{}
        phrases := make([]string, 0, maxEvents)
        for _, ev := range events {
                typ := strings.TrimSpace(ev.Type)
                if typ == "" {
                        continue
                }
                counts[typ]++
                if len(phrases) < maxEvents {
                        phrases = append(phrases, describeEvent(ev))
                }
        }
        // Make the summary stable and compact: show most common first.
        type kv struct {
                k string
                v int
        }
        var ordered []kv
        for k, v := range counts {
                ordered = append(ordered, kv{k: k, v: v})
        }
        sort.SliceStable(ordered, func(i, j int) bool {
                if ordered[i].v != ordered[j].v {
                        return ordered[i].v > ordered[j].v
                }
                return ordered[i].k < ordered[j].k
        })
        _ = ordered // reserved for future refinement

        // Dedupe adjacent identical phrases.
        out := make([]string, 0, len(phrases))
        seen := map[string]bool{}
        for _, p := range phrases {
                p = strings.TrimSpace(p)
                if p == "" {
                        continue
                }
                if seen[p] {
                        continue
                }
                seen[p] = true
                out = append(out, p)
        }
        if len(out) == 0 {
                return "", counts, nil
        }
        if len(events) > maxEvents {
                out = append(out, fmt.Sprintf("+%d more", len(events)-maxEvents))
        }
        return strings.Join(out, "; "), counts, nil
}

func parseAddedJSONLines(diff string) []stagedEvent {
        lines := strings.Split(diff, "\n")
        out := make([]stagedEvent, 0)
        for _, ln := range lines {
                ln = strings.TrimRight(ln, "\r")
                if !strings.HasPrefix(ln, "+") {
                        continue
                }
                if strings.HasPrefix(ln, "+++") {
                        continue
                }
                raw := strings.TrimSpace(strings.TrimPrefix(ln, "+"))
                if !strings.HasPrefix(raw, "{") {
                        continue
                }
                var ev stagedEvent
                if err := json.Unmarshal([]byte(raw), &ev); err != nil {
                        continue
                }
                out = append(out, ev)
        }
        return out
}

func describeEvent(ev stagedEvent) string {
        typ := strings.TrimSpace(ev.Type)
        if strings.HasSuffix(typ, ".merge") {
                return "merge"
        }
        switch typ {
        case "item.create":
                var p struct {
                        Title string `json:"title"`
                }
                _ = json.Unmarshal(ev.Payload, &p)
                if strings.TrimSpace(p.Title) != "" {
                        return `create "` + strings.TrimSpace(p.Title) + `"`
                }
                return "create item"
        case "item.set_title":
                var p struct {
                        Title string `json:"title"`
                }
                _ = json.Unmarshal(ev.Payload, &p)
                if strings.TrimSpace(p.Title) != "" {
                        return `title "` + strings.TrimSpace(p.Title) + `"`
                }
                return "title item"
        case "item.set_status":
                var p struct {
                        StatusID string `json:"statusId"`
                }
                _ = json.Unmarshal(ev.Payload, &p)
                if strings.TrimSpace(p.StatusID) != "" {
                        return "status " + strings.TrimSpace(p.StatusID)
                }
                return "status"
        case "item.set_description":
                return "edit description"
        case "item.set_parent":
                return "move item"
        case "item.move":
                return "reorder item"
        case "comment.add":
                var p struct {
                        Body string `json:"body"`
                }
                _ = json.Unmarshal(ev.Payload, &p)
                body := strings.TrimSpace(p.Body)
                if body != "" {
                        // Keep commit messages readable at a glance.
                        body = strings.ReplaceAll(body, "\n", " ")
                        if len(body) > 40 {
                                body = body[:40] + "…"
                        }
                        return `comment "` + body + `"`
                }
                return "comment"
        case "worklog.add":
                return "worklog"
        default:
                // Compact fallback: strip entity prefix.
                if i := strings.Index(typ, "."); i > 0 {
                        return strings.ReplaceAll(typ[i+1:], "_", " ")
                }
                return strings.ReplaceAll(typ, "_", " ")
        }
}
