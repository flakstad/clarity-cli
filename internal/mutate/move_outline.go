package mutate

import (
	"errors"
	"strings"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/perm"
	"clarity-cli/internal/statusutil"
	"clarity-cli/internal/store"
)

func MoveItemToOutline(db *store.DB, actorID, itemID, toOutlineID, statusOverride string, applyStatusToInvalidSubtree bool, now time.Time) (bool, map[string]any, error) {
	itemID = strings.TrimSpace(itemID)
	toOutlineID = strings.TrimSpace(toOutlineID)
	actorID = strings.TrimSpace(actorID)
	statusOverride = strings.TrimSpace(statusOverride)
	if db == nil || itemID == "" || toOutlineID == "" {
		return false, nil, nil
	}
	if actorID == "" {
		return false, nil, errors.New("missing actor")
	}

	it, ok := db.FindItem(itemID)
	if !ok || it == nil || it.Archived {
		return false, nil, errors.New("item not found")
	}
	o, ok := db.FindOutline(toOutlineID)
	if !ok || o == nil || o.Archived {
		return false, nil, errors.New("outline not found")
	}

	if applyStatusToInvalidSubtree {
		if statusOverride != "" && !validateStatusIDForMove(*o, statusOverride) {
			return false, nil, errors.New("invalid status id for target outline")
		}
	}

	ids := collectSubtreeItemIDs(db, it.ID)
	if len(ids) == 0 {
		return false, nil, nil
	}

	for _, id := range ids {
		x, ok := db.FindItem(id)
		if !ok || x == nil {
			continue
		}
		if !perm.CanEditItem(db, actorID, x) {
			return false, nil, errors.New("permission denied")
		}
	}

	changed := false
	for _, id := range ids {
		x, ok := db.FindItem(id)
		if !ok || x == nil {
			continue
		}

		nextStatus := strings.TrimSpace(x.StatusID)
		if id == it.ID && (applyStatusToInvalidSubtree || statusOverride != "") {
			nextStatus = statusOverride
		}
		if nextStatus != "" && !validateStatusIDForMove(*o, nextStatus) {
			if applyStatusToInvalidSubtree {
				nextStatus = statusOverride
			} else {
				return false, nil, errors.New("invalid status id for target outline; pick a compatible status")
			}
		}

		if strings.TrimSpace(x.OutlineID) != strings.TrimSpace(o.ID) {
			x.OutlineID = o.ID
			changed = true
		}
		if strings.TrimSpace(x.ProjectID) != strings.TrimSpace(o.ProjectID) {
			x.ProjectID = o.ProjectID
			changed = true
		}
		if strings.TrimSpace(x.StatusID) != strings.TrimSpace(nextStatus) {
			x.StatusID = nextStatus
			changed = true
		}
		if !x.UpdatedAt.Equal(now) {
			x.UpdatedAt = now
			changed = true
		}
	}

	if it.ParentID != nil {
		it.ParentID = nil
		changed = true
	}
	nextRank := nextRootRank(db, o.ID)
	if strings.TrimSpace(it.Rank) != strings.TrimSpace(nextRank) {
		it.Rank = nextRank
		changed = true
	}

	if !changed {
		return false, nil, nil
	}
	return true, map[string]any{"to": o.ID, "status": strings.TrimSpace(it.StatusID)}, nil
}

func validateStatusIDForMove(outline model.Outline, statusID string) bool {
	sid := strings.TrimSpace(statusID)
	if sid == "" {
		return true
	}
	if len(outline.StatusDefs) == 0 {
		switch strings.ToLower(sid) {
		case "todo", "doing", "done":
			return true
		default:
			return false
		}
	}
	return statusutil.ValidateStatusID(outline, sid)
}

func collectSubtreeItemIDs(db *store.DB, rootID string) []string {
	rootID = strings.TrimSpace(rootID)
	if db == nil || rootID == "" {
		return nil
	}
	out := []string{}
	seen := map[string]bool{}
	var walk func(id string)
	walk = func(id string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, id)
		for _, ch := range db.ChildrenOf(id) {
			walk(ch.ID)
		}
	}
	walk(rootID)
	return out
}

func nextRootRank(db *store.DB, outlineID string) string {
	outlineID = strings.TrimSpace(outlineID)
	if db == nil || outlineID == "" {
		r, _ := store.RankInitial()
		return r
	}
	max := ""
	for _, it := range db.Items {
		if it.Archived {
			continue
		}
		if strings.TrimSpace(it.OutlineID) != outlineID {
			continue
		}
		if it.ParentID != nil {
			continue
		}
		r := strings.TrimSpace(it.Rank)
		if r == "" {
			continue
		}
		if max == "" || r > max {
			max = r
		}
	}
	if max == "" {
		r, _ := store.RankInitial()
		return r
	}
	r, err := store.RankAfter(max)
	if err != nil {
		r2, _ := store.RankInitial()
		return r2
	}
	return r
}
