package store

import (
        "encoding/json"
        "fmt"
        "os"
        "sort"
        "strings"
        "time"
)

// Event log backend selection.
//
// Default: legacy JSONL file (events.jsonl).
// Opt-in: set CLARITY_EVENTLOG=sqlite to use the SQLite event log.
const envEventLogBackend = "CLARITY_EVENTLOG"

type EventLogBackend string

const (
        EventLogBackendJSONL  EventLogBackend = "jsonl"
        EventLogBackendSQLite EventLogBackend = "sqlite"
)

// EntityKind is the per-entity stream identifier used by the v1 event contract.
// These map to future NATS subjects and central ingestion routing.
type EntityKind string

const (
        EntityKindActor   EntityKind = "actor"
        EntityKindProject EntityKind = "project"
        EntityKindOutline EntityKind = "outline"
        EntityKindItem    EntityKind = "item"
        EntityKindComment EntityKind = "comment"
        EntityKindWorklog EntityKind = "worklog"

        // Legacy/other (not yet in the v1 “entity boundary” decision, but present today).
        EntityKindDep EntityKind = "dep"
)

// EventV1 is the durable, per-entity ordered event envelope stored in SQLite.
//
// NOTE: This is intentionally broader than the legacy model.Event used for JSONL.
// The store maps EventV1 -> model.Event when serving existing CLI/TUI callers.
type EventV1 struct {
        EventID     string `json:"eventId"`
        WorkspaceID string `json:"workspaceId"`
        ReplicaID   string `json:"replicaId"`

        EntityKind EntityKind `json:"entityKind"`
        EntityID   string     `json:"entityId"`
        EntitySeq  int64      `json:"entitySeq"`

        Type    string   `json:"type"`
        Parents []string `json:"parents,omitempty"`

        IssuedAt time.Time       `json:"issuedAt"`
        ActorID  string          `json:"actorId"`
        Payload  json.RawMessage `json:"payload"`

        LocalStatus       string  `json:"localStatus"`                 // e.g. "local"
        ServerStatus      string  `json:"serverStatus"`                // "pending"|"accepted"|"rejected"
        RejectionReason   *string `json:"rejectionReason,omitempty"`   // set when rejected
        PublishedAtUnixMs *int64  `json:"publishedAtUnixMs,omitempty"` // outbox marker (future)
}

func (s Store) eventLogBackend() EventLogBackend {
        v := strings.ToLower(strings.TrimSpace(getenv(envEventLogBackend)))
        switch v {
        case string(EventLogBackendJSONL):
                return EventLogBackendJSONL
        case string(EventLogBackendSQLite):
                return EventLogBackendSQLite
        default:
                // Auto-detect: if a Git-backed events directory exists, prefer JSONL.
                // Otherwise default to SQLite (current stable behavior).
                if s.hasJSONLEvents() {
                        return EventLogBackendJSONL
                }
                return EventLogBackendSQLite
        }
}

func (s Store) hasJSONLEvents() bool {
        dir := s.eventsDir()
        entries, err := os.ReadDir(dir)
        if err != nil {
                return false
        }
        var names []string
        for _, ent := range entries {
                if ent.IsDir() {
                        continue
                }
                name := ent.Name()
                if !strings.HasPrefix(name, "events") || !strings.HasSuffix(name, ".jsonl") {
                        continue
                }
                names = append(names, name)
        }
        sort.Strings(names)
        return len(names) > 0
}

// inferEntityKindFromType maps existing event type prefixes to v1 entity kinds.
// This preserves current call sites that only provide (type, entityID).
func inferEntityKindFromType(typ string) EntityKind {
        prefix := strings.TrimSpace(typ)
        if prefix == "" {
                return ""
        }
        if i := strings.Index(prefix, "."); i >= 0 {
                prefix = prefix[:i]
        }
        switch prefix {
        case "item":
                return EntityKindItem
        case "project":
                return EntityKindProject
        case "outline":
                return EntityKindOutline
        case "comment":
                return EntityKindComment
        case "worklog":
                return EntityKindWorklog
        case "identity":
                // identity.* events operate on actor entities.
                return EntityKindActor
        case "dep":
                return EntityKindDep
        default:
                return EntityKind(prefix)
        }
}

func (k EntityKind) valid() bool {
        return strings.TrimSpace(string(k)) != ""
}

func (k EntityKind) String() string { return string(k) }

func formatErrEventContract(msg string, args ...any) error {
        return fmt.Errorf("event contract: "+msg, args...)
}
