package tui

func (m appModel) appendEvent(actorID string, eventType string, entityID string, payload any) error {
        if err := m.store.AppendEvent(actorID, eventType, entityID, payload); err != nil {
                return err
        }
        if m.autoCommit != nil {
                m.autoCommit.Notify()
        }
        return nil
}
