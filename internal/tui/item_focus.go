package tui

// itemPageFocus represents which control is focused on the full-screen item page.
// The zero value is a valid focus (title), which keeps state initialization simple.
type itemPageFocus int

const (
        itemFocusTitle itemPageFocus = iota
        itemFocusStatus
        itemFocusPriority
        itemFocusDescription
        itemFocusComments
        itemFocusWorklog
        itemFocusHistory
)

var itemFocusOrder = []itemPageFocus{
        itemFocusTitle,
        itemFocusStatus,
        itemFocusPriority,
        itemFocusDescription,
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
