package tui

type actionSpec struct {
	key   string
	label string
}

func addActionSpecs(dst map[string]actionPanelAction, specs []actionSpec) {
	for _, s := range specs {
		dst[s.key] = actionPanelAction{
			label: s.label,
			kind:  actionPanelActionExec,
		}
	}
}

var itemActionsCoreSpecs = []actionSpec{
	{key: "e", label: "Edit title"},
	{key: "D", label: "Edit description"},
	{key: "p", label: "Toggle priority"},
	{key: "o", label: "Toggle on hold"},
	{key: "A", label: "Assign…"},
	{key: "t", label: "Tags…"},
	{key: "d", label: "Set due"},
	{key: "s", label: "Set schedule"},
	{key: " ", label: "Change status"},
	{key: "C", label: "Add comment"},
	{key: "w", label: "Add worklog"},
	{key: "y", label: "Copy item ref (includes --workspace)"},
	{key: "Y", label: "Copy CLI show command (includes --workspace)"},
	{key: "V", label: "Duplicate item"},
	{key: "m", label: "Move…"},
	{key: "r", label: "Archive item"},
}

var itemActionsItemViewExtrasSpecs = []actionSpec{
	{key: "l", label: "Open links…"},
	{key: "u", label: "Attach file…"},
	{key: "H", label: "View history"},
}

var itemActionsItemViewReadOnlySpecs = []actionSpec{
	{key: "l", label: "Open links…"},
	{key: "V", label: "Duplicate item"},
	{key: "y", label: "Copy item ref (includes --workspace)"},
	{key: "Y", label: "Copy CLI show command (includes --workspace)"},
	{key: "H", label: "View history"},
}
