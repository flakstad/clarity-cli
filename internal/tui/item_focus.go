package tui

// itemPageFocus represents which control is focused on the full-screen item page.
// The zero value is a valid focus (title), which keeps state initialization simple.
type itemPageFocus int

const (
        itemFocusTitle itemPageFocus = iota
        itemFocusStatus
        itemFocusAssigned
        itemFocusTags
        itemFocusPriority
        itemFocusDescription
        itemFocusParent
        itemFocusChildren
        itemFocusComments
        itemFocusWorklog
        itemFocusHistory
)

var itemFocusOrder = []itemPageFocus{
        itemFocusTitle,
        itemFocusStatus,
        itemFocusAssigned,
        itemFocusTags,
        itemFocusPriority,
        itemFocusDescription,
        itemFocusParent,
        itemFocusChildren,
        itemFocusComments,
        itemFocusWorklog,
        itemFocusHistory,
}

func (f itemPageFocus) next() itemPageFocus {
        for i := 0; i < len(itemFocusOrder); i++ {
                if itemFocusOrder[i] == f {
                        return itemFocusOrder[(i+1)%len(itemFocusOrder)]
                }
        }
        return itemFocusTitle
}

func (f itemPageFocus) prev() itemPageFocus {
        for i := 0; i < len(itemFocusOrder); i++ {
                if itemFocusOrder[i] == f {
                        return itemFocusOrder[(i-1+len(itemFocusOrder))%len(itemFocusOrder)]
                }
        }
        return itemFocusTitle
}

func (f itemPageFocus) nextForItem(hasParent bool) itemPageFocus {
        start := 0
        for i := 0; i < len(itemFocusOrder); i++ {
                if itemFocusOrder[i] == f {
                        start = i
                        break
                }
        }
        for step := 1; step <= len(itemFocusOrder); step++ {
                cand := itemFocusOrder[(start+step)%len(itemFocusOrder)]
                if cand == itemFocusParent && !hasParent {
                        continue
                }
                return cand
        }
        return itemFocusTitle
}

func (f itemPageFocus) prevForItem(hasParent bool) itemPageFocus {
        start := 0
        for i := 0; i < len(itemFocusOrder); i++ {
                if itemFocusOrder[i] == f {
                        start = i
                        break
                }
        }
        for step := 1; step <= len(itemFocusOrder); step++ {
                cand := itemFocusOrder[(start-step+len(itemFocusOrder))%len(itemFocusOrder)]
                if cand == itemFocusParent && !hasParent {
                        continue
                }
                return cand
        }
        return itemFocusTitle
}
