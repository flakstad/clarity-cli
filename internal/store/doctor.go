package store

import (
        "errors"
        "fmt"
        "strings"
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
        lines, err := ReadEventsV1Lines(dir)
        if err != nil {
                return DoctorReport{
                        Issues: []DoctorIssue{{
                                Level:   DoctorIssueLevelError,
                                Code:    "events_read_failed",
                                Message: err.Error(),
                        }},
                }
        }
        if len(lines) == 0 {
                return DoctorReport{Issues: []DoctorIssue{}}
        }

        var issues []DoctorIssue

        seen := map[string]EventV1Line{}
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

        if issues == nil {
                issues = []DoctorIssue{}
        }
        return DoctorReport{Issues: issues}
}

var ErrDoctorIssuesFound = errors.New("doctor: issues found")
