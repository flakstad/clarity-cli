package tui

import (
	"fmt"
	"sort"
	"strings"

	"clarity-cli/internal/store"

	"github.com/charmbracelet/bubbles/list"
)

func activityCommentsRootID(itemID string) string { return "__comments__:" + strings.TrimSpace(itemID) }
func activityWorklogRootID(itemID string) string  { return "__worklog__:" + strings.TrimSpace(itemID) }

func injectItemActivityRows(items []list.Item, rootItemID string, collapsed map[string]bool, db *store.DB, contentW int) []list.Item {
	rootItemID = strings.TrimSpace(rootItemID)
	if rootItemID == "" || db == nil {
		return items
	}
	if collapsed == nil {
		collapsed = map[string]bool{}
	}

	rootIdx := -1
	rootDepth := 0
	for i := range items {
		if it, ok := items[i].(outlineRowItem); ok {
			if strings.TrimSpace(it.row.item.ID) == rootItemID {
				rootIdx = i
				rootDepth = it.row.depth
				break
			}
		}
	}
	if rootIdx < 0 {
		return items
	}

	insertAt := rootIdx + 1
	for insertAt < len(items) {
		desc, ok := items[insertAt].(outlineDescRowItem)
		if !ok {
			break
		}
		if strings.TrimSpace(desc.parentID) != rootItemID {
			break
		}
		insertAt++
	}

	activity := buildItemActivityOutlineRows(db, rootItemID, collapsed, rootDepth+1, contentW)
	if len(activity) == 0 {
		return items
	}

	out := make([]list.Item, 0, len(items)+len(activity))
	out = append(out, items[:insertAt]...)
	out = append(out, activity...)
	out = append(out, items[insertAt:]...)
	return out
}

func buildItemActivityOutlineRows(db *store.DB, itemID string, collapsed map[string]bool, baseDepth int, contentW int) []list.Item {
	itemID = strings.TrimSpace(itemID)
	if db == nil || itemID == "" {
		return nil
	}
	if baseDepth < 0 {
		baseDepth = 0
	}
	if collapsed == nil {
		collapsed = map[string]bool{}
	}
	if contentW <= 0 {
		contentW = 80
	}

	out := make([]list.Item, 0, 64)

	// Comments.
	comments := db.CommentsForItem(itemID)
	commentRows := buildCommentThreadRows(comments)
	commentKids := map[string]int{}
	for _, c := range comments {
		if c.ReplyToCommentID == nil {
			continue
		}
		p := strings.TrimSpace(*c.ReplyToCommentID)
		if p == "" {
			continue
		}
		commentKids[p]++
	}
	commentsRootID := activityCommentsRootID(itemID)
	if len(comments) > 0 {
		if _, ok := collapsed[commentsRootID]; !ok {
			collapsed[commentsRootID] = true
		}
		out = append(out, outlineActivityRowItem{
			id:          commentsRootID,
			itemID:      itemID,
			kind:        outlineActivityCommentsRoot,
			depth:       baseDepth,
			label:       fmt.Sprintf("Comments (%d)", len(comments)),
			hasChildren: true,
			collapsed:   collapsed[commentsRootID],
		})
		if !collapsed[commentsRootID] {
			skipDepth := -1
			for _, r := range commentRows {
				if skipDepth >= 0 && r.Depth > skipDepth {
					continue
				}
				if skipDepth >= 0 && r.Depth <= skipDepth {
					skipDepth = -1
				}

				c := r.Comment
				cid := strings.TrimSpace(c.ID)
				if cid == "" {
					continue
				}
				hasChildren := commentKids[cid] > 0
				bodyMD := strings.TrimSpace(commentMarkdownWithAttachments(db, c))
				hasDescription := bodyMD != ""
				if hasChildren || hasDescription {
					if _, ok := collapsed[cid]; !ok {
						collapsed[cid] = true
					}
				}

				label := fmt.Sprintf("%s %s", fmtTS(c.CreatedAt), actorAtLabel(db, c.AuthorID))
				commentDepth := baseDepth + 1 + r.Depth

				out = append(out, outlineActivityRowItem{
					id:             cid,
					itemID:         itemID,
					kind:           outlineActivityComment,
					depth:          commentDepth,
					label:          label,
					commentID:      cid,
					hasChildren:    hasChildren,
					hasDescription: hasDescription,
					collapsed:      collapsed[cid],
				})

				if hasDescription && !collapsed[cid] {
					descDepth := commentDepth
					leadW := (2 * descDepth) + 2
					avail := contentW - leadW
					if avail < 0 {
						avail = 0
					}
					descLines := outlineDescriptionLinesMarkdown(bodyMD, avail)
					for _, line := range descLines {
						out = append(out, outlineDescRowItem{parentID: cid, depth: descDepth, line: line})
					}
				}

				if (hasChildren || hasDescription) && collapsed[cid] {
					skipDepth = r.Depth
				}
			}
		}
	}

	// Worklog.
	worklog := db.WorklogForItem(itemID)
	sort.SliceStable(worklog, func(i, j int) bool { return worklog[i].CreatedAt.After(worklog[j].CreatedAt) })
	worklogRootID := activityWorklogRootID(itemID)
	if len(worklog) > 0 {
		if _, ok := collapsed[worklogRootID]; !ok {
			collapsed[worklogRootID] = true
		}
		out = append(out, outlineActivityRowItem{
			id:          worklogRootID,
			itemID:      itemID,
			kind:        outlineActivityWorklogRoot,
			depth:       baseDepth,
			label:       fmt.Sprintf("My worklog (%d)", len(worklog)),
			hasChildren: true,
			collapsed:   collapsed[worklogRootID],
		})
		if !collapsed[worklogRootID] {
			for _, w := range worklog {
				wid := strings.TrimSpace(w.ID)
				if wid == "" {
					continue
				}
				body := strings.TrimSpace(w.Body)
				hasDescription := body != ""
				if hasDescription {
					if _, ok := collapsed[wid]; !ok {
						collapsed[wid] = true
					}
				}
				label := fmt.Sprintf("%s %s", fmtTS(w.CreatedAt), actorAtLabel(db, w.AuthorID))
				worklogDepth := baseDepth + 1
				out = append(out, outlineActivityRowItem{
					id:             wid,
					itemID:         itemID,
					kind:           outlineActivityWorklogEntry,
					depth:          worklogDepth,
					label:          label,
					worklogID:      wid,
					hasDescription: hasDescription,
					collapsed:      collapsed[wid],
				})
				if hasDescription && !collapsed[wid] {
					descDepth := worklogDepth
					leadW := (2 * descDepth) + 2
					avail := contentW - leadW
					if avail < 0 {
						avail = 0
					}
					descLines := outlineDescriptionLinesMarkdown(body, avail)
					for _, line := range descLines {
						out = append(out, outlineDescRowItem{parentID: wid, depth: descDepth, line: line})
					}
				}
			}
		}
	}

	return out
}
