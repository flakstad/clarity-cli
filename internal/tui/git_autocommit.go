package tui

import "strings"

func (m appModel) appendEvent(actorID string, eventType string, entityID string, payload any) error {
        if err := m.store.AppendEvent(actorID, eventType, entityID, payload); err != nil {
                return err
        }
        if m.autoCommit != nil {
                label := actorID
                if m.db != nil {
                        if a, ok := m.db.FindActor(actorID); ok && strings.TrimSpace(a.Name) != "" {
                                label = strings.TrimSpace(a.Name)
                        }
                }
                m.autoCommit.Notify(label)
        }
        return nil
}
