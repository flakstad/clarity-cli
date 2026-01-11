package tui

import (
	"strings"
	"testing"
	"time"

	"clarity-cli/internal/model"
	"clarity-cli/internal/store"
)

func TestRenderItemDetailInteractive_CommentMetaAndBodyIndentMatch(t *testing.T) {
	actorID := "act-human"
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	db := &store.DB{
		CurrentActorID: actorID,
		Actors:         []model.Actor{{ID: actorID, Kind: model.ActorKindHuman, Name: "A"}},
		Projects: []model.Project{{
			ID:        "proj-a",
			Name:      "Project A",
			CreatedBy: actorID,
			CreatedAt: now,
		}},
		Outlines: []model.Outline{{
			ID:         "out-a",
			ProjectID:  "proj-a",
			StatusDefs: store.DefaultOutlineStatusDefs(),
			CreatedBy:  actorID,
			CreatedAt:  now,
		}},
		Items: []model.Item{{
			ID:           "item-a",
			ProjectID:    "proj-a",
			OutlineID:    "out-a",
			Rank:         "h",
			Title:        "Title",
			StatusID:     "todo",
			OwnerActorID: actorID,
			CreatedBy:    actorID,
			CreatedAt:    now,
			UpdatedAt:    now,
		}},
	}

	c1ID := "c1"
	c2ID := "c2"
	db.Comments = []model.Comment{
		{
			ID:        c1ID,
			ItemID:    "item-a",
			AuthorID:  actorID,
			Body:      "Root comment",
			CreatedAt: now.Add(1 * time.Minute),
		},
		{
			ID:               c2ID,
			ItemID:           "item-a",
			AuthorID:         actorID,
			ReplyToCommentID: &c1ID,
			Body:             "Reply 1",
			CreatedAt:        now.Add(2 * time.Minute),
		},
		{
			ID:               "c3",
			ItemID:           "item-a",
			AuthorID:         actorID,
			ReplyToCommentID: &c2ID,
			Body:             "Deep reply",
			CreatedAt:        now.Add(3 * time.Minute),
		},
	}

	out := renderItemDetailInteractive(
		db,
		db.Outlines[0],
		db.Items[0],
		80, 40,
		itemFocusComments,
		nil,  // events
		0, 0, // childIdx/childOff
		0,    // attachmentIdx
		2,    // commentIdx (deep reply)
		0, 0, // worklogIdx/historyIdx
		0, // scroll
	)

	lines := strings.Split(out, "\n")
	findLine := func(needle string) int {
		for i := range lines {
			if strings.Contains(stripANSIEscapes(lines[i]), needle) {
				return i
			}
		}
		return -1
	}

	bodyIdx := findLine("Deep reply")
	if bodyIdx <= 0 {
		t.Fatalf("expected to find body line %q in output", "Deep reply")
	}

	metaIdx := bodyIdx - 1
	metaPlain := stripANSIEscapes(lines[metaIdx])
	bodyPlain := stripANSIEscapes(lines[bodyIdx])
	if !strings.Contains(metaPlain, "↳ ") {
		t.Fatalf("expected meta line to contain reply arrow; got %q", metaPlain)
	}

	leadingSpaces := func(s string) int {
		n := 0
		for n < len(s) && s[n] == ' ' {
			n++
		}
		return n
	}

	if got, want := leadingSpaces(metaPlain), leadingSpaces(bodyPlain); got != want {
		t.Fatalf("expected comment meta/body indent to match; meta=%d body=%d\nmeta=%q\nbody=%q", got, want, metaPlain, bodyPlain)
	}
}
