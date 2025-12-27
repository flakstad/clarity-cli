package mutate

import "fmt"

type NotFoundError struct {
        Kind string
        ID   string
}

func (e NotFoundError) Error() string {
        return fmt.Sprintf("%s not found: %s", e.Kind, e.ID)
}

type OwnerOnlyError struct {
        ActorID      string
        OwnerActorID string
        ItemID       string
}

func (e OwnerOnlyError) Error() string {
        // Keep this generic; CLI/TUI can wrap with more specific phrasing.
        return "owner-only"
}
