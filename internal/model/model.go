package model

import "time"

type ActorKind string

const (
        ActorKindHuman ActorKind = "human"
        ActorKindAgent ActorKind = "agent"
)

type Actor struct {
        ID     string    `json:"id"`
        Kind   ActorKind `json:"kind"`
        Name   string    `json:"name"`
        UserID *string   `json:"userId,omitempty"`
}

type Project struct {
        ID        string    `json:"id"`
        Name      string    `json:"name"`
        CreatedBy string    `json:"createdBy"`
        CreatedAt time.Time `json:"createdAt"`
        Archived  bool      `json:"archived"`
}

type OutlineStatusDef struct {
        ID         string `json:"id"`
        Label      string `json:"label"`
        IsEndState bool   `json:"isEndState"`
}

type Outline struct {
        ID         string             `json:"id"`
        ProjectID  string             `json:"projectId"`
        Name       *string            `json:"name,omitempty"`
        StatusDefs []OutlineStatusDef `json:"statusDefs"`
        CreatedBy  string             `json:"createdBy"`
        CreatedAt  time.Time          `json:"createdAt"`
        Archived   bool               `json:"archived"`
}

type Item struct {
        ID        string `json:"id"`
        ProjectID string `json:"projectId"`
        OutlineID string `json:"outlineId"`

        ParentID *string `json:"parentId,omitempty"`
        Rank     string  `json:"rank,omitempty"`

        Title       string    `json:"title"`
        Description string    `json:"description,omitempty"`
        StatusID    string    `json:"status,omitempty"`
        Priority    bool      `json:"priority"`
        OnHold      bool      `json:"onHold"`
        Due         *DateTime `json:"due,omitempty"`
        Schedule    *DateTime `json:"schedule,omitempty"`

        // Legacy fields (migrated to Due/Schedule on load).
        LegacyDueAt       *time.Time `json:"dueAt,omitempty"`
        LegacyScheduledAt *time.Time `json:"scheduledAt,omitempty"`
        Tags              []string   `json:"tags,omitempty"`
        Archived          bool       `json:"archived"`

        OwnerActorID    string  `json:"ownerActorId"`
        AssignedActorID *string `json:"assignedActorId,omitempty"`

        // Ownership delegation: when ownership is transferred (typically by assignment),
        // the previous owner may retain edit rights for a short grace period.
        OwnerDelegatedFrom *string    `json:"ownerDelegatedFrom,omitempty"`
        OwnerDelegatedAt   *time.Time `json:"ownerDelegatedAt,omitempty"`

        CreatedBy string    `json:"createdBy"`
        CreatedAt time.Time `json:"createdAt"`
        UpdatedAt time.Time `json:"updatedAt"`
}

// DateTime represents an optional time attached to a date.
// If Time is nil, the value is date-only (no time semantics).
type DateTime struct {
        Date string  `json:"date"`           // YYYY-MM-DD
        Time *string `json:"time,omitempty"` // HH:MM
}

type DependencyType string

const (
        DependencyBlocks  DependencyType = "blocks"
        DependencyRelated DependencyType = "related"
)

type Dependency struct {
        ID         string         `json:"id"`
        FromItemID string         `json:"fromItemId"`
        ToItemID   string         `json:"toItemId"`
        Type       DependencyType `json:"type"`
        CreatedBy  string         `json:"createdBy"`
        CreatedAt  time.Time      `json:"createdAt"`

        // Legacy fields for migration.
        LegacyFromTaskID string `json:"fromTaskId,omitempty"`
        LegacyToTaskID   string `json:"toTaskId,omitempty"`
}

type Comment struct {
        ID               string    `json:"id"`
        ItemID           string    `json:"itemId"`
        AuthorID         string    `json:"authorId"`
        ReplyToCommentID *string   `json:"replyToCommentId,omitempty"`
        Body             string    `json:"body"`
        CreatedAt        time.Time `json:"createdAt"`

        LegacyTaskID string `json:"taskId,omitempty"`
}

type WorklogEntry struct {
        ID        string    `json:"id"`
        ItemID    string    `json:"itemId"`
        AuthorID  string    `json:"authorId"`
        Body      string    `json:"body"`
        CreatedAt time.Time `json:"createdAt"`

        LegacyTaskID string `json:"taskId,omitempty"`
}

type Event struct {
        ID       string    `json:"id"`
        TS       time.Time `json:"ts"`
        ActorID  string    `json:"actorId"`
        Type     string    `json:"type"`
        EntityID string    `json:"entityId"`
        Payload  any       `json:"payload"`
}
