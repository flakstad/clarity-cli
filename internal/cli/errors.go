package cli

import "fmt"

type notFoundError struct {
        kind string
        id   string
}

func (e notFoundError) Error() string {
        return fmt.Sprintf("%s not found: %s", e.kind, e.id)
}

func errNotFound(kind, id string) error {
        return notFoundError{kind: kind, id: id}
}

type ownerOnlyError struct {
        actorID string
        ownerID string
        taskID  string
}

func (e ownerOnlyError) Error() string {
        return fmt.Sprintf("permission denied: actor %s is not owner %s for item %s", e.actorID, e.ownerID, e.taskID)
}

func errorsOwnerOnly(actorID, ownerID, taskID string) error {
        return ownerOnlyError{actorID: actorID, ownerID: ownerID, taskID: taskID}
}
