package store

import (
        "bufio"
        "bytes"
        "context"
        "encoding/json"
        "errors"
        "fmt"
        "os"
        "path/filepath"
        "sort"
        "strings"

        "clarity-cli/internal/gitrepo"
)

type DoctorIssueLevel string

const (
        DoctorIssueLevelError DoctorIssueLevel = "error"
        DoctorIssueLevelWarn  DoctorIssueLevel = "warn"
)

type DoctorIssue struct {
        Level   DoctorIssueLevel `json:"level"`
        Code    string           `json:"code"`
        Message string           `json:"message"`
        Path    string           `json:"path,omitempty"`
        Line    int              `json:"line,omitempty"`

        EventID     string `json:"eventId,omitempty"`
        ReplicaID   string `json:"replicaId,omitempty"`
        WorkspaceID string `json:"workspaceId,omitempty"`
        EntityKind  string `json:"entityKind,omitempty"`
        EntityID    string `json:"entityId,omitempty"`
        Type        string `json:"type,omitempty"`
}

type DoctorReport struct {
        Issues []DoctorIssue `json:"issues"`
}

func (r DoctorReport) HasErrors() bool {
        for _, it := range r.Issues {
                if it.Level == DoctorIssueLevelError {
                        return true
                }
        }
        return false
}

func DoctorEventsV1(dir string) DoctorReport {
        st := Store{Dir: dir}
        wsRoot := st.workspaceRoot()

        var issues []DoctorIssue

        // If this workspace lives in a Git repo, report in-progress/conflict state.
        if gs, err := gitrepo.GetStatus(context.Background(), wsRoot); err == nil && gs.IsRepo {
                if gs.Unmerged || gs.InProgress {
                        issues = append(issues, DoctorIssue{
                                Level:   DoctorIssueLevelError,
                                Code:    "git_in_progress",
                                Message: "git merge/rebase in progress; resolve conflicts before writing events",
                        })
                }
        }

        // Load committed workspaceId from meta/workspace.json (if present) for consistency checks.
        metaWorkspaceID := ""
        metaPath := filepath.Join(wsRoot, "meta", "workspace.json")
        if b, err := os.ReadFile(metaPath); err == nil && len(bytes.TrimSpace(b)) > 0 {
                var m WorkspaceMetaFile
                if err := json.Unmarshal(b, &m); err != nil {
                        issues = append(issues, DoctorIssue{
                                Level:   DoctorIssueLevelError,
                                Code:    "workspace_meta_invalid_json",
                                Message: err.Error(),
                                Path:    metaPath,
                                Line:    1,
                        })
                } else {
                        metaWorkspaceID = strings.TrimSpace(m.WorkspaceID)
                        if metaWorkspaceID == "" {
                                issues = append(issues, DoctorIssue{
                                        Level:   DoctorIssueLevelError,
                                        Code:    "workspace_meta_missing_id",
                                        Message: "meta/workspace.json: empty workspaceId",
                                        Path:    metaPath,
                                        Line:    1,
                                })
                        }
                }
        }

        // Scan events JSONL files, capturing parse errors as issues (not as a hard failure).
        lines := []EventV1Line{}
        eventsDir := st.eventsDir()
        entries, err := os.ReadDir(eventsDir)
        if err != nil {
                if errors.Is(err, os.ErrNotExist) {
                        return DoctorReport{Issues: issuesOrEmpty(issues)}
                }
                return DoctorReport{
                        Issues: []DoctorIssue{{
                                Level:   DoctorIssueLevelError,
                                Code:    "events_read_failed",
                                Message: err.Error(),
                        }},
                }
        }
        var paths []string
        for _, ent := range entries {
                if ent.IsDir() {
                        continue
                }
                name := ent.Name()
                if !strings.HasPrefix(name, "events") || !strings.HasSuffix(name, ".jsonl") {
                        continue
                }
                paths = append(paths, filepath.Join(eventsDir, name))
        }
        sort.Strings(paths)

        for _, p := range paths {
                replicaFromFile := replicaIDFromShardFilename(filepath.Base(p))

                f, err := os.Open(p)
                if err != nil {
                        issues = append(issues, DoctorIssue{
                                Level:   DoctorIssueLevelError,
                                Code:    "events_open_failed",
                                Message: err.Error(),
                                Path:    p,
                        })
                        continue
                }

                sc := bufio.NewScanner(f)
                sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

                lineNo := 0
                for sc.Scan() {
                        lineNo++
                        b := bytes.TrimSpace(sc.Bytes())
                        if len(b) == 0 {
                                continue
                        }

                        // Common merge markers.
                        if bytes.HasPrefix(b, []byte("<<<<<<<")) || bytes.HasPrefix(b, []byte("=======")) || bytes.HasPrefix(b, []byte(">>>>>>>")) {
                                issues = append(issues, DoctorIssue{
                                        Level:   DoctorIssueLevelError,
                                        Code:    "merge_marker",
                                        Message: "git merge conflict marker in events log",
                                        Path:    p,
                                        Line:    lineNo,
                                })
                                continue
                        }

                        var ev EventV1
                        if err := json.Unmarshal(b, &ev); err != nil {
                                issues = append(issues, DoctorIssue{
                                        Level:   DoctorIssueLevelError,
                                        Code:    "malformed_json",
                                        Message: err.Error(),
                                        Path:    p,
                                        Line:    lineNo,
                                })
                                continue
                        }

                        if replicaFromFile != "" && strings.TrimSpace(ev.ReplicaID) != "" && strings.TrimSpace(ev.ReplicaID) != replicaFromFile {
                                issues = append(issues, DoctorIssue{
                                        Level:     DoctorIssueLevelError,
                                        Code:      "replica_id_mismatch",
                                        Message:   fmt.Sprintf("replicaId %q does not match shard filename %q", strings.TrimSpace(ev.ReplicaID), replicaFromFile),
                                        Path:      p,
                                        Line:      lineNo,
                                        EventID:   strings.TrimSpace(ev.EventID),
                                        ReplicaID: strings.TrimSpace(ev.ReplicaID),
                                        Type:      strings.TrimSpace(ev.Type),
                                })
                        }
                        if metaWorkspaceID != "" && strings.TrimSpace(ev.WorkspaceID) != "" && strings.TrimSpace(ev.WorkspaceID) != metaWorkspaceID {
                                issues = append(issues, DoctorIssue{
                                        Level:       DoctorIssueLevelError,
                                        Code:        "workspace_id_mismatch",
                                        Message:     fmt.Sprintf("workspaceId %q does not match meta/workspace.json %q", strings.TrimSpace(ev.WorkspaceID), metaWorkspaceID),
                                        Path:        p,
                                        Line:        lineNo,
                                        EventID:     strings.TrimSpace(ev.EventID),
                                        WorkspaceID: strings.TrimSpace(ev.WorkspaceID),
                                        Type:        strings.TrimSpace(ev.Type),
                                })
                        }

                        lines = append(lines, EventV1Line{Path: p, Line: lineNo, Event: ev})
                }
                _ = f.Close()
                if err := sc.Err(); err != nil {
                        issues = append(issues, DoctorIssue{
                                Level:   DoctorIssueLevelError,
                                Code:    "events_scan_failed",
                                Message: err.Error(),
                                Path:    p,
                        })
                }
        }
        if len(lines) == 0 {
                return DoctorReport{Issues: issuesOrEmpty(issues)}
        }

        seen := map[string]EventV1Line{}
        type entKey struct {
                kind string
                id   string
        }
        type entAgg struct {
                ids        map[string]EventV1Line
                parentIDs  map[string]struct{}
                parentRefs map[string]EventV1Line // parentEventId -> referencing child line (first seen)
        }
        globalEventToEnt := map[string]entKey{}
        aggs := map[entKey]*entAgg{}
        for _, l := range lines {
                ev := l.Event

                typ := strings.TrimSpace(ev.Type)
                if typ == "" {
                        issues = append(issues, DoctorIssue{
                                Level:   DoctorIssueLevelError,
                                Code:    "missing_type",
                                Message: "missing event type",
                                Path:    l.Path,
                                Line:    l.Line,
                        })
                }
                if strings.TrimSpace(ev.EventID) == "" {
                        issues = append(issues, DoctorIssue{
                                Level:   DoctorIssueLevelError,
                                Code:    "missing_event_id",
                                Message: "missing eventId",
                                Path:    l.Path,
                                Line:    l.Line,
                                Type:    typ,
                        })
                }
                if strings.TrimSpace(ev.ReplicaID) == "" {
                        issues = append(issues, DoctorIssue{
                                Level:   DoctorIssueLevelError,
                                Code:    "missing_replica_id",
                                Message: "missing replicaId",
                                Path:    l.Path,
                                Line:    l.Line,
                                EventID: strings.TrimSpace(ev.EventID),
                                Type:    typ,
                        })
                }
                if strings.TrimSpace(ev.WorkspaceID) == "" {
                        issues = append(issues, DoctorIssue{
                                Level:   DoctorIssueLevelError,
                                Code:    "missing_workspace_id",
                                Message: "missing workspaceId",
                                Path:    l.Path,
                                Line:    l.Line,
                                EventID: strings.TrimSpace(ev.EventID),
                                Type:    typ,
                        })
                }
                if strings.TrimSpace(ev.ActorID) == "" {
                        issues = append(issues, DoctorIssue{
                                Level:   DoctorIssueLevelError,
                                Code:    "missing_actor_id",
                                Message: "missing actorId",
                                Path:    l.Path,
                                Line:    l.Line,
                                EventID: strings.TrimSpace(ev.EventID),
                                Type:    typ,
                        })
                }
                if ev.IssuedAt.IsZero() {
                        issues = append(issues, DoctorIssue{
                                Level:   DoctorIssueLevelError,
                                Code:    "missing_issued_at",
                                Message: "missing issuedAt",
                                Path:    l.Path,
                                Line:    l.Line,
                                EventID: strings.TrimSpace(ev.EventID),
                                Type:    typ,
                        })
                }
                if strings.TrimSpace(ev.EntityID) == "" || !ev.EntityKind.valid() {
                        issues = append(issues, DoctorIssue{
                                Level:      DoctorIssueLevelError,
                                Code:       "missing_entity",
                                Message:    "missing entity kind/id",
                                Path:       l.Path,
                                Line:       l.Line,
                                EventID:    strings.TrimSpace(ev.EventID),
                                Type:       typ,
                                EntityID:   strings.TrimSpace(ev.EntityID),
                                EntityKind: ev.EntityKind.String(),
                        })
                }

                // Basic contract sanity: type prefix -> entity kind.
                if typ != "" && ev.EntityKind.valid() {
                        want := inferEntityKindFromType(typ)
                        if want.valid() && want != ev.EntityKind {
                                issues = append(issues, DoctorIssue{
                                        Level:      DoctorIssueLevelError,
                                        Code:       "entity_kind_mismatch",
                                        Message:    fmt.Sprintf("entityKind %q does not match type %q (expected %q)", ev.EntityKind, typ, want),
                                        Path:       l.Path,
                                        Line:       l.Line,
                                        EventID:    strings.TrimSpace(ev.EventID),
                                        ReplicaID:  strings.TrimSpace(ev.ReplicaID),
                                        EntityKind: ev.EntityKind.String(),
                                        EntityID:   strings.TrimSpace(ev.EntityID),
                                        Type:       typ,
                                })
                        }
                }

                // Duplicate detection.
                if strings.TrimSpace(ev.EventID) != "" && strings.TrimSpace(ev.ReplicaID) != "" {
                        key := strings.TrimSpace(ev.ReplicaID) + "/" + strings.TrimSpace(ev.EventID)
                        if prev, ok := seen[key]; ok {
                                issues = append(issues, DoctorIssue{
                                        Level:       DoctorIssueLevelError,
                                        Code:        "duplicate_event",
                                        Message:     fmt.Sprintf("duplicate event: %s (also in %s:%d)", key, prev.Path, prev.Line),
                                        Path:        l.Path,
                                        Line:        l.Line,
                                        EventID:     strings.TrimSpace(ev.EventID),
                                        ReplicaID:   strings.TrimSpace(ev.ReplicaID),
                                        WorkspaceID: strings.TrimSpace(ev.WorkspaceID),
                                        EntityKind:  ev.EntityKind.String(),
                                        EntityID:    strings.TrimSpace(ev.EntityID),
                                        Type:        typ,
                                })
                        } else {
                                seen[key] = l
                        }
                }

                // Parent invariants / fork detection bookkeeping.
                entID := strings.TrimSpace(ev.EntityID)
                if entID != "" && ev.EntityKind.valid() {
                        k := entKey{kind: ev.EntityKind.String(), id: entID}
                        a := aggs[k]
                        if a == nil {
                                a = &entAgg{
                                        ids:        map[string]EventV1Line{},
                                        parentIDs:  map[string]struct{}{},
                                        parentRefs: map[string]EventV1Line{},
                                }
                                aggs[k] = a
                        }

                        evID := strings.TrimSpace(ev.EventID)
                        if evID != "" {
                                a.ids[evID] = l
                                if prevK, ok := globalEventToEnt[evID]; ok && (prevK.kind != k.kind || prevK.id != k.id) {
                                        issues = append(issues, DoctorIssue{
                                                Level:      DoctorIssueLevelError,
                                                Code:       "event_id_reused",
                                                Message:    fmt.Sprintf("eventId %q appears in multiple entities: %s/%s and %s/%s", evID, prevK.kind, prevK.id, k.kind, k.id),
                                                Path:       l.Path,
                                                Line:       l.Line,
                                                EventID:    evID,
                                                EntityKind: k.kind,
                                                EntityID:   k.id,
                                                Type:       typ,
                                        })
                                } else {
                                        globalEventToEnt[evID] = k
                                }
                        }

                        if len(ev.Parents) > 1 {
                                issues = append(issues, DoctorIssue{
                                        Level:      DoctorIssueLevelWarn,
                                        Code:       "multiple_parents",
                                        Message:    fmt.Sprintf("event has %d parents (merge events not yet supported in v1 tooling)", len(ev.Parents)),
                                        Path:       l.Path,
                                        Line:       l.Line,
                                        EventID:    evID,
                                        ReplicaID:  strings.TrimSpace(ev.ReplicaID),
                                        EntityKind: k.kind,
                                        EntityID:   k.id,
                                        Type:       typ,
                                })
                        }

                        for _, p := range ev.Parents {
                                p = strings.TrimSpace(p)
                                if p == "" {
                                        continue
                                }
                                if evID != "" && p == evID {
                                        issues = append(issues, DoctorIssue{
                                                Level:      DoctorIssueLevelError,
                                                Code:       "self_parent",
                                                Message:    "event lists itself as a parent",
                                                Path:       l.Path,
                                                Line:       l.Line,
                                                EventID:    evID,
                                                ReplicaID:  strings.TrimSpace(ev.ReplicaID),
                                                EntityKind: k.kind,
                                                EntityID:   k.id,
                                                Type:       typ,
                                        })
                                        continue
                                }
                                a.parentIDs[p] = struct{}{}
                                if _, ok := a.parentRefs[p]; !ok {
                                        a.parentRefs[p] = l
                                }
                        }
                }

                // Payload should be present and valid JSON.
                if len(ev.Payload) == 0 {
                        issues = append(issues, DoctorIssue{
                                Level:     DoctorIssueLevelWarn,
                                Code:      "empty_payload",
                                Message:   "empty payload (expected JSON object)",
                                Path:      l.Path,
                                Line:      l.Line,
                                EventID:   strings.TrimSpace(ev.EventID),
                                ReplicaID: strings.TrimSpace(ev.ReplicaID),
                                Type:      typ,
                        })
                }
        }

        // Fork detection + parent integrity checks.
        for k, a := range aggs {
                if a == nil {
                        continue
                }
                for parentID, ref := range a.parentRefs {
                        if _, ok := a.ids[parentID]; ok {
                                continue
                        }
                        if pk, ok := globalEventToEnt[parentID]; ok && (pk.kind != k.kind || pk.id != k.id) {
                                issues = append(issues, DoctorIssue{
                                        Level:      DoctorIssueLevelError,
                                        Code:       "parent_cross_entity",
                                        Message:    fmt.Sprintf("parent %q belongs to a different entity (%s/%s)", parentID, pk.kind, pk.id),
                                        Path:       ref.Path,
                                        Line:       ref.Line,
                                        EventID:    strings.TrimSpace(ref.Event.EventID),
                                        ReplicaID:  strings.TrimSpace(ref.Event.ReplicaID),
                                        EntityKind: k.kind,
                                        EntityID:   k.id,
                                        Type:       strings.TrimSpace(ref.Event.Type),
                                })
                                continue
                        }
                        issues = append(issues, DoctorIssue{
                                Level:      DoctorIssueLevelError,
                                Code:       "missing_parent_event",
                                Message:    fmt.Sprintf("parent %q not found in this entity stream", parentID),
                                Path:       ref.Path,
                                Line:       ref.Line,
                                EventID:    strings.TrimSpace(ref.Event.EventID),
                                ReplicaID:  strings.TrimSpace(ref.Event.ReplicaID),
                                EntityKind: k.kind,
                                EntityID:   k.id,
                                Type:       strings.TrimSpace(ref.Event.Type),
                        })
                }

                var heads []EventV1Line
                for id, l := range a.ids {
                        if _, ok := a.parentIDs[id]; ok {
                                continue
                        }
                        heads = append(heads, l)
                }
                sort.Slice(heads, func(i, j int) bool {
                        if heads[i].Path != heads[j].Path {
                                return heads[i].Path < heads[j].Path
                        }
                        return heads[i].Line < heads[j].Line
                })

                if len(a.ids) > 0 && len(heads) == 0 {
                        issues = append(issues, DoctorIssue{
                                Level:      DoctorIssueLevelError,
                                Code:       "no_heads",
                                Message:    "entity stream has no heads (cycle or corrupt parents)",
                                EntityKind: k.kind,
                                EntityID:   k.id,
                        })
                        continue
                }
                if len(heads) > 1 {
                        issues = append(issues, DoctorIssue{
                                Level:      DoctorIssueLevelError,
                                Code:       "fork_detected",
                                Message:    fmt.Sprintf("entity has %d heads (requires explicit merge)", len(heads)),
                                EntityKind: k.kind,
                                EntityID:   k.id,
                        })
                        for _, h := range heads {
                                issues = append(issues, DoctorIssue{
                                        Level:      DoctorIssueLevelError,
                                        Code:       "fork_head",
                                        Message:    "entity head",
                                        Path:       h.Path,
                                        Line:       h.Line,
                                        EventID:    strings.TrimSpace(h.Event.EventID),
                                        ReplicaID:  strings.TrimSpace(h.Event.ReplicaID),
                                        EntityKind: k.kind,
                                        EntityID:   k.id,
                                        Type:       strings.TrimSpace(h.Event.Type),
                                })
                        }
                }
        }

        return DoctorReport{Issues: issuesOrEmpty(issues)}
}

var ErrDoctorIssuesFound = errors.New("doctor: issues found")

func issuesOrEmpty(xs []DoctorIssue) []DoctorIssue {
        if xs == nil {
                return []DoctorIssue{}
        }
        return xs
}

func replicaIDFromShardFilename(name string) string {
        name = strings.TrimSpace(name)
        if name == "" || name == "events.jsonl" {
                return ""
        }
        // events.<replica>.jsonl
        if !strings.HasPrefix(name, "events.") || !strings.HasSuffix(name, ".jsonl") {
                return ""
        }
        core := strings.TrimSuffix(strings.TrimPrefix(name, "events."), ".jsonl")
        return strings.TrimSpace(core)
}
