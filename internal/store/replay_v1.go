package store

import (
        "encoding/json"
        "fmt"
        "sort"
        "strings"
        "time"

        "clarity-cli/internal/model"
)

type ReplayResult struct {
        DB           *DB
        AppliedCount int
        SkippedCount int
        SkippedTypes map[string]int
}

// ReplayEventsV1 materializes workspace state from the Git-backed EventV1 JSONL logs.
//
// Notes:
// - This is a best-effort V1 reducer. Unknown event types are skipped (counted), not fatal.
// - Ordering uses (issuedAt, eventId, replicaId, path, line) for determinism.
func ReplayEventsV1(dir string) (ReplayResult, error) {
        lines, err := ReadEventsV1Lines(dir)
        if err != nil {
                return ReplayResult{}, err
        }

        sort.Slice(lines, func(i, j int) bool {
                a := lines[i].Event
                b := lines[j].Event
                if !a.IssuedAt.Equal(b.IssuedAt) {
                        return a.IssuedAt.Before(b.IssuedAt)
                }
                if a.EventID != b.EventID {
                        return a.EventID < b.EventID
                }
                if a.ReplicaID != b.ReplicaID {
                        return a.ReplicaID < b.ReplicaID
                }
                if lines[i].Path != lines[j].Path {
                        return lines[i].Path < lines[j].Path
                }
                return lines[i].Line < lines[j].Line
        })

        db := &DB{
                Version:  1,
                NextIDs:  map[string]int{},
                Actors:   []model.Actor{},
                Projects: []model.Project{},
                Outlines: []model.Outline{},
                Items:    []model.Item{},
                Deps:     []model.Dependency{},
                Comments: []model.Comment{},
                Worklog:  []model.WorklogEntry{},
        }

        res := ReplayResult{
                DB:           db,
                AppliedCount: 0,
                SkippedCount: 0,
                SkippedTypes: map[string]int{},
        }

        for _, l := range lines {
                applied, err := applyEventV1(db, l.Event)
                if err != nil {
                        return ReplayResult{}, fmt.Errorf("%s:%d: %w", l.Path, l.Line, err)
                }
                if applied {
                        res.AppliedCount++
                } else {
                        res.SkippedCount++
                        res.SkippedTypes[strings.TrimSpace(l.Event.Type)]++
                }
        }

        return res, nil
}

func applyEventV1(db *DB, ev EventV1) (bool, error) {
        if db == nil {
                return false, nil
        }
        typ := strings.TrimSpace(ev.Type)
        switch typ {
        case "identity.create":
                var p struct {
                        Name   string  `json:"name"`
                        Kind   string  `json:"kind"`
                        UserID *string `json:"userId"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                id := strings.TrimSpace(ev.EntityID)
                if id == "" {
                        return false, nil
                }
                kind, err := NormalizeActorKind(p.Kind)
                if err != nil {
                        return false, err
                }
                if a, ok := db.FindActor(id); ok && a != nil {
                        a.Name = strings.TrimSpace(p.Name)
                        a.Kind = kind
                        a.UserID = p.UserID
                } else {
                        db.Actors = append(db.Actors, model.Actor{
                                ID:     id,
                                Kind:   kind,
                                Name:   strings.TrimSpace(p.Name),
                                UserID: p.UserID,
                        })
                }
                return true, nil

        case "identity.seed":
                // Local helper event used when seeding identity across workspaces.
                // Not part of the materialized workspace state.
                return false, nil

        case "identity.use":
                // Local-only in the Git-backed model; ignore during materialization.
                return false, nil

        case "project.create":
                var p model.Project
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                if _, ok := db.FindProject(p.ID); ok {
                        return true, nil
                }
                db.Projects = append(db.Projects, p)
                return true, nil

        case "project.rename":
                var p struct {
                        Name string `json:"name"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                proj, ok := db.FindProject(ev.EntityID)
                if !ok || proj == nil {
                        return true, nil
                }
                name := strings.TrimSpace(p.Name)
                if name != "" {
                        proj.Name = name
                }
                return true, nil

        case "project.archive":
                var p struct {
                        Archived bool `json:"archived"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                proj, ok := db.FindProject(ev.EntityID)
                if !ok || proj == nil {
                        return true, nil
                }
                proj.Archived = p.Archived
                return true, nil

        case "outline.create":
                var o model.Outline
                if err := json.Unmarshal(ev.Payload, &o); err != nil {
                        return false, err
                }
                if _, ok := db.FindOutline(o.ID); ok {
                        return true, nil
                }
                if o.StatusDefs == nil {
                        o.StatusDefs = DefaultOutlineStatusDefs()
                }
                db.Outlines = append(db.Outlines, o)
                return true, nil

        case "outline.rename":
                var p struct {
                        Name *string `json:"name"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                o, ok := db.FindOutline(ev.EntityID)
                if !ok || o == nil {
                        return true, nil
                }
                if p.Name == nil {
                        o.Name = nil
                        return true, nil
                }
                tmp := strings.TrimSpace(*p.Name)
                if tmp == "" {
                        o.Name = nil
                        return true, nil
                }
                o.Name = &tmp
                return true, nil

        case "outline.set_description":
                var p struct {
                        Description string `json:"description"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                o, ok := db.FindOutline(ev.EntityID)
                if !ok || o == nil {
                        return true, nil
                }
                o.Description = strings.TrimSpace(p.Description)
                return true, nil

        case "outline.archive":
                var p struct {
                        Archived bool `json:"archived"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                o, ok := db.FindOutline(ev.EntityID)
                if !ok || o == nil {
                        return true, nil
                }
                o.Archived = p.Archived
                return true, nil

        case "outline.status.add":
                var p struct {
                        ID           string `json:"id"`
                        Label        string `json:"label"`
                        IsEndState   bool   `json:"isEndState"`
                        RequiresNote bool   `json:"requiresNote"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                o, ok := db.FindOutline(ev.EntityID)
                if !ok || o == nil {
                        return true, nil
                }
                o.StatusDefs = append(o.StatusDefs, model.OutlineStatusDef{
                        ID:           strings.TrimSpace(p.ID),
                        Label:        strings.TrimSpace(p.Label),
                        IsEndState:   p.IsEndState,
                        RequiresNote: p.RequiresNote,
                })
                return true, nil

        case "outline.status.remove":
                var p struct {
                        ID string `json:"id"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                o, ok := db.FindOutline(ev.EntityID)
                if !ok || o == nil {
                        return true, nil
                }
                id := strings.TrimSpace(p.ID)
                var next []model.OutlineStatusDef
                for _, d := range o.StatusDefs {
                        if strings.TrimSpace(d.ID) == id {
                                continue
                        }
                        next = append(next, d)
                }
                o.StatusDefs = next
                return true, nil

        case "outline.status.update":
                // Payload variants:
                // - TUI: {"id": "...", "label": "...", "isEndState": true, "requiresNote": false, "ts": "..."}
                // - CLI: {"key":"id-or-label","label":"...","end":true,"notEnd":false,"requireNote":true,"noRequireNote":false,"ts":"..."}
                var p struct {
                        ID           string `json:"id"`
                        Key          string `json:"key"`
                        Label        string `json:"label"`
                        IsEndState   *bool  `json:"isEndState"`
                        RequiresNote *bool  `json:"requiresNote"`
                        End          *bool  `json:"end"`
                        NotEnd       *bool  `json:"notEnd"`
                        RequireNote  *bool  `json:"requireNote"`
                        NoRequire    *bool  `json:"noRequireNote"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                o, ok := db.FindOutline(ev.EntityID)
                if !ok || o == nil {
                        return true, nil
                }

                key := strings.TrimSpace(p.ID)
                if key == "" {
                        key = strings.TrimSpace(p.Key)
                }
                if key == "" {
                        return true, nil
                }

                idx := -1
                for i := range o.StatusDefs {
                        if strings.TrimSpace(o.StatusDefs[i].ID) == key || strings.TrimSpace(o.StatusDefs[i].Label) == key {
                                idx = i
                                break
                        }
                }
                if idx < 0 {
                        return true, nil
                }

                if strings.TrimSpace(p.Label) != "" {
                        o.StatusDefs[idx].Label = strings.TrimSpace(p.Label)
                }
                if p.IsEndState != nil {
                        o.StatusDefs[idx].IsEndState = *p.IsEndState
                }
                if p.End != nil && *p.End {
                        o.StatusDefs[idx].IsEndState = true
                }
                if p.NotEnd != nil && *p.NotEnd {
                        o.StatusDefs[idx].IsEndState = false
                }
                if p.RequiresNote != nil {
                        o.StatusDefs[idx].RequiresNote = *p.RequiresNote
                }
                if p.RequireNote != nil && *p.RequireNote {
                        o.StatusDefs[idx].RequiresNote = true
                }
                if p.NoRequire != nil && *p.NoRequire {
                        o.StatusDefs[idx].RequiresNote = false
                }
                return true, nil

        case "outline.status.reorder":
                var p struct {
                        Labels []string `json:"labels"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                o, ok := db.FindOutline(ev.EntityID)
                if !ok || o == nil {
                        return true, nil
                }
                // Reorder by label list (labels are unique in an outline); keep unknown ones at end.
                byLabel := map[string]model.OutlineStatusDef{}
                for _, d := range o.StatusDefs {
                        byLabel[strings.TrimSpace(d.Label)] = d
                }
                var out []model.OutlineStatusDef
                seen := map[string]bool{}
                for _, id := range p.Labels {
                        id = strings.TrimSpace(id)
                        if id == "" || seen[id] {
                                continue
                        }
                        if d, ok := byLabel[id]; ok {
                                out = append(out, d)
                                seen[id] = true
                        }
                }
                for _, d := range o.StatusDefs {
                        id := strings.TrimSpace(d.Label)
                        if id == "" || seen[id] {
                                continue
                        }
                        out = append(out, d)
                }
                o.StatusDefs = out
                return true, nil

        case "item.create":
                var it model.Item
                if err := json.Unmarshal(ev.Payload, &it); err != nil {
                        return false, err
                }
                if _, ok := db.FindItem(it.ID); ok {
                        return true, nil
                }
                db.Items = append(db.Items, it)
                return true, nil

        case "item.created":
                // Legacy alias (used in older tests / backups); treat as no-op during replay.
                return false, nil

        case "item.set_title":
                var p struct {
                        Title string `json:"title"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                it.Title = p.Title
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.set_description":
                var p struct {
                        Description string `json:"description"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                it.Description = p.Description
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.set_status":
                var p struct {
                        To     string `json:"to"`
                        Status string `json:"status"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                if strings.TrimSpace(p.To) != "" {
                        it.StatusID = strings.TrimSpace(p.To)
                } else {
                        it.StatusID = strings.TrimSpace(p.Status)
                }
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.set_priority":
                var p struct {
                        Priority bool `json:"priority"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                it.Priority = p.Priority
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.set_on_hold":
                var p struct {
                        OnHold bool `json:"onHold"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                it.OnHold = p.OnHold
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.set_due":
                var p struct {
                        Due *model.DateTime `json:"due"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                it.Due = p.Due
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.set_schedule":
                var p struct {
                        Schedule *model.DateTime `json:"schedule"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                it.Schedule = p.Schedule
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.set_assign":
                var p struct {
                        AssignedActorID *string `json:"assignedActorId"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                next := ""
                if p.AssignedActorID != nil {
                        next = strings.TrimSpace(*p.AssignedActorID)
                }
                if next == "" {
                        it.AssignedActorID = nil
                        it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                        return true, nil
                }
                tmp := next
                it.AssignedActorID = &tmp
                if strings.TrimSpace(it.OwnerActorID) != next {
                        prev := strings.TrimSpace(it.OwnerActorID)
                        if prev != "" {
                                it.OwnerDelegatedFrom = &prev
                                t := issuedOrNow(ev.IssuedAt)
                                it.OwnerDelegatedAt = &t
                        }
                        it.OwnerActorID = next
                }
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.archive":
                var p struct {
                        Archived bool `json:"archived"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                it.Archived = p.Archived
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.tags_add":
                var p struct {
                        Tag string `json:"tag"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                tag := strings.TrimSpace(p.Tag)
                if tag == "" {
                        return true, nil
                }
                for _, existing := range it.Tags {
                        if existing == tag {
                                return true, nil
                        }
                }
                it.Tags = append(it.Tags, tag)
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.tags_remove":
                var p struct {
                        Tag string `json:"tag"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                tag := strings.TrimSpace(p.Tag)
                var next []string
                for _, existing := range it.Tags {
                        if existing == tag {
                                continue
                        }
                        next = append(next, existing)
                }
                it.Tags = next
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.tags_set":
                var p struct {
                        Tags []string `json:"tags"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                it.Tags = p.Tags
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.move":
                var p struct {
                        Before    string            `json:"before"`
                        After     string            `json:"after"`
                        Rank      string            `json:"rank"`
                        Rebalance map[string]string `json:"rebalance"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }

                // Prefer explicit rank updates.
                if strings.TrimSpace(p.Rank) != "" {
                        it.Rank = strings.TrimSpace(p.Rank)
                        it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                } else if strings.TrimSpace(p.Before) != "" || strings.TrimSpace(p.After) != "" {
                        // Legacy move events that only specify relative position.
                        refID := strings.TrimSpace(p.Before)
                        mode := "before"
                        if refID == "" {
                                refID = strings.TrimSpace(p.After)
                                mode = "after"
                        }
                        sibs := siblingItemsForReplay(db, it.OutlineID, it.ParentID, ev.EntityID)
                        refIdx := indexOfItemPtr(sibs, refID)
                        if refIdx >= 0 {
                                insertAt := refIdx
                                if mode == "after" {
                                        insertAt = refIdx + 1
                                }
                                res, err := PlanReorderRanks(sibs, it.ID, insertAt)
                                if err != nil {
                                        return false, err
                                }
                                for id, r := range res.RankByID {
                                        x, ok := db.FindItem(id)
                                        if !ok || x == nil || strings.TrimSpace(r) == "" {
                                                continue
                                        }
                                        x.Rank = strings.TrimSpace(r)
                                        x.UpdatedAt = issuedOrNow(ev.IssuedAt)
                                }
                        }
                }
                for id, r := range p.Rebalance {
                        it, ok := db.FindItem(id)
                        if !ok || it == nil {
                                continue
                        }
                        if strings.TrimSpace(r) == "" {
                                continue
                        }
                        it.Rank = strings.TrimSpace(r)
                        it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                }
                return true, nil

        case "item.set_parent":
                var p struct {
                        Parent    string            `json:"parent"`
                        Before    string            `json:"before"`
                        After     string            `json:"after"`
                        Rank      string            `json:"rank"`
                        Rebalance map[string]string `json:"rebalance"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                parent := strings.TrimSpace(p.Parent)
                if parent == "" || parent == "none" {
                        it.ParentID = nil
                } else {
                        tmp := parent
                        it.ParentID = &tmp
                }
                if strings.TrimSpace(p.Rank) != "" {
                        it.Rank = strings.TrimSpace(p.Rank)
                } else if strings.TrimSpace(p.Before) != "" || strings.TrimSpace(p.After) != "" {
                        refID := strings.TrimSpace(p.Before)
                        mode := "before"
                        if refID == "" {
                                refID = strings.TrimSpace(p.After)
                                mode = "after"
                        }
                        sibs := siblingItemsForReplay(db, it.OutlineID, it.ParentID, it.ID)
                        refIdx := indexOfItemPtr(sibs, refID)
                        if refIdx >= 0 {
                                insertAt := refIdx
                                if mode == "after" {
                                        insertAt = refIdx + 1
                                }
                                res, err := PlanReorderRanks(sibs, it.ID, insertAt)
                                if err != nil {
                                        return false, err
                                }
                                for id, r := range res.RankByID {
                                        x, ok := db.FindItem(id)
                                        if !ok || x == nil || strings.TrimSpace(r) == "" {
                                                continue
                                        }
                                        x.Rank = strings.TrimSpace(r)
                                        x.UpdatedAt = issuedOrNow(ev.IssuedAt)
                                }
                        }
                }
                for id, r := range p.Rebalance {
                        x, ok := db.FindItem(id)
                        if !ok || x == nil {
                                continue
                        }
                        if strings.TrimSpace(r) == "" {
                                continue
                        }
                        x.Rank = strings.TrimSpace(r)
                        x.UpdatedAt = issuedOrNow(ev.IssuedAt)
                }
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.indent":
                var p struct {
                        Under string `json:"under"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                under := strings.TrimSpace(p.Under)
                if under == "" {
                        return true, nil
                }
                tmp := under
                it.ParentID = &tmp
                it.Rank = nextSiblingRank(db, it.OutlineID, it.ParentID)
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.outdent":
                // Move to parent's parent and place after parent when possible (best-effort).
                var p struct {
                        FromParent string `json:"fromParent"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                parentID := strings.TrimSpace(p.FromParent)
                if parentID == "" && it.ParentID != nil {
                        parentID = strings.TrimSpace(*it.ParentID)
                }
                if parentID == "" {
                        return true, nil
                }
                parent, ok := db.FindItem(parentID)
                if !ok || parent == nil {
                        it.ParentID = nil
                        it.Rank = nextSiblingRank(db, it.OutlineID, nil)
                        it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                        return true, nil
                }
                it.ParentID = parent.ParentID
                it.Rank = nextSiblingRank(db, it.OutlineID, it.ParentID)
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "item.move_outline":
                var p struct {
                        To     string `json:"to"`
                        Status string `json:"status"`
                }
                if err := json.Unmarshal(ev.Payload, &p); err != nil {
                        return false, err
                }
                it, ok := db.FindItem(ev.EntityID)
                if !ok || it == nil {
                        return true, nil
                }
                to := strings.TrimSpace(p.To)
                if to != "" {
                        it.OutlineID = to
                }
                it.ParentID = nil
                it.StatusID = strings.TrimSpace(p.Status)
                it.Rank = nextSiblingRank(db, it.OutlineID, nil)
                it.UpdatedAt = issuedOrNow(ev.IssuedAt)
                return true, nil

        case "dep.add":
                var d model.Dependency
                if err := json.Unmarshal(ev.Payload, &d); err != nil {
                        return false, err
                }
                if _, ok := findDepByID(db, d.ID); ok {
                        return true, nil
                }
                db.Deps = append(db.Deps, d)
                return true, nil

        case "comment.add":
                var c model.Comment
                if err := json.Unmarshal(ev.Payload, &c); err != nil {
                        return false, err
                }
                if _, ok := findCommentByID(db, c.ID); ok {
                        return true, nil
                }
                db.Comments = append(db.Comments, c)
                return true, nil

        case "worklog.add":
                var w model.WorklogEntry
                if err := json.Unmarshal(ev.Payload, &w); err != nil {
                        return false, err
                }
                if _, ok := findWorklogByID(db, w.ID); ok {
                        return true, nil
                }
                db.Worklog = append(db.Worklog, w)
                return true, nil

        default:
                return false, nil
        }
}

func issuedOrNow(t time.Time) time.Time {
        if t.IsZero() {
                return time.Now().UTC()
        }
        return t.UTC()
}

func nextSiblingRank(db *DB, outlineID string, parentID *string) string {
        max := ""
        for i := range db.Items {
                it := &db.Items[i]
                if it.OutlineID != outlineID {
                        continue
                }
                if (it.ParentID == nil) != (parentID == nil) {
                        continue
                }
                if it.ParentID != nil && parentID != nil && *it.ParentID != *parentID {
                        continue
                }
                r := strings.TrimSpace(it.Rank)
                if r != "" && r > max {
                        max = r
                }
        }
        if max == "" {
                r, err := RankInitial()
                if err != nil {
                        return "h"
                }
                return r
        }
        r, err := RankAfter(max)
        if err != nil {
                return max + "0"
        }
        return r
}

func siblingItemsForReplay(db *DB, outlineID string, parentID *string, movedID string) []*model.Item {
        var out []*model.Item
        for i := range db.Items {
                it := &db.Items[i]
                if strings.TrimSpace(it.OutlineID) != strings.TrimSpace(outlineID) {
                        continue
                }
                if it.Archived && it.ID != movedID {
                        continue
                }
                if (it.ParentID == nil) != (parentID == nil) {
                        continue
                }
                if it.ParentID != nil && parentID != nil && strings.TrimSpace(*it.ParentID) != strings.TrimSpace(*parentID) {
                        continue
                }
                out = append(out, it)
        }
        sort.Slice(out, func(i, j int) bool {
                return strings.TrimSpace(out[i].Rank) < strings.TrimSpace(out[j].Rank)
        })
        return out
}

func indexOfItemPtr(items []*model.Item, id string) int {
        id = strings.TrimSpace(id)
        if id == "" {
                return -1
        }
        for i := range items {
                if strings.TrimSpace(items[i].ID) == id {
                        return i
                }
        }
        return -1
}

func findDepByID(db *DB, id string) (*model.Dependency, bool) {
        id = strings.TrimSpace(id)
        if id == "" {
                return nil, false
        }
        for i := range db.Deps {
                if strings.TrimSpace(db.Deps[i].ID) == id {
                        return &db.Deps[i], true
                }
        }
        return nil, false
}

func findCommentByID(db *DB, id string) (*model.Comment, bool) {
        id = strings.TrimSpace(id)
        if id == "" {
                return nil, false
        }
        for i := range db.Comments {
                if strings.TrimSpace(db.Comments[i].ID) == id {
                        return &db.Comments[i], true
                }
        }
        return nil, false
}

func findWorklogByID(db *DB, id string) (*model.WorklogEntry, bool) {
        id = strings.TrimSpace(id)
        if id == "" {
                return nil, false
        }
        for i := range db.Worklog {
                if strings.TrimSpace(db.Worklog[i].ID) == id {
                        return &db.Worklog[i], true
                }
        }
        return nil, false
}
