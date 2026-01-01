package cli

import (
        "context"
        "fmt"
        "strings"
        "time"

        "clarity-cli/internal/gitrepo"
        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func autoSyncWorkspaceBestEffort(cmd *cobra.Command, app *App) error {
        dir := strings.TrimSpace(app.Dir)
        if dir == "" {
                return nil
        }

        st := store.Store{Dir: dir}
        if !st.IsJSONLWorkspace() {
                return nil
        }
        if !gitrepo.AutoCommitEnabled() {
                return nil
        }

        ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
        defer cancel()

        gs, err := gitrepo.GetStatus(ctx, dir)
        if err != nil || !gs.IsRepo || gs.Unmerged || gs.InProgress {
                return nil
        }

        db, err := st.Load()
        if err != nil || db == nil {
                return nil
        }
        actorID := strings.TrimSpace(app.ActorID)
        if actorID == "" {
                actorID = strings.TrimSpace(db.CurrentActorID)
        }
        actorLabel := strings.TrimSpace(actorID)
        if a, ok := db.FindActor(actorID); ok && a != nil && strings.TrimSpace(a.Name) != "" {
                actorLabel = strings.TrimSpace(a.Name)
        }

        committed, pushed, err := gitrepo.AutoCommitAndPush(ctx, dir, actorLabel)
        if err != nil {
                // Do not fail the command; this is best-effort.
                fmt.Fprintf(cmd.ErrOrStderr(), "warning: auto-sync failed: %v\n", err)
                return nil
        }
        _ = committed
        _ = pushed
        return nil
}
