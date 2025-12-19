package store

import (
        "time"

        "clarity-cli/internal/model"
)

func DefaultOutlineStatusDefs() []model.OutlineStatusDef {
        return []model.OutlineStatusDef{
                {ID: "todo", Label: "TODO", IsEndState: false},
                {ID: "doing", Label: "DOING", IsEndState: false},
                {ID: "done", Label: "DONE", IsEndState: true},
        }
}

func (db *DB) EnsureDefaultOutline(projectID, actorID string, nextID func(prefix string) string) string {
        for _, o := range db.Outlines {
                if o.ProjectID == projectID {
                        return o.ID
                }
        }
        id := nextID("out")
        db.Outlines = append(db.Outlines, model.Outline{
                ID:         id,
                ProjectID:  projectID,
                Name:       nil,
                StatusDefs: DefaultOutlineStatusDefs(),
                CreatedBy:  actorID,
                CreatedAt:  time.Now().UTC(),
        })
        return id
}

func (db *DB) StatusDef(outlineID, statusID string) (*model.OutlineStatusDef, bool) {
        o, ok := db.FindOutline(outlineID)
        if !ok {
                return nil, false
        }
        for i := range o.StatusDefs {
                if o.StatusDefs[i].ID == statusID {
                        return &o.StatusDefs[i], true
                }
        }
        return nil, false
}
