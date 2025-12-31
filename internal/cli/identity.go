package cli

import (
        "errors"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newIdentityCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "identity",
                Short: "Manage local Clarity identities (actors)",
        }

        cmd.AddCommand(newIdentityCreateCmd(app))
        cmd.AddCommand(newIdentityAgentCmd(app))
        cmd.AddCommand(newIdentityUseCmd(app))
        cmd.AddCommand(newIdentityListCmd(app))
        cmd.AddCommand(newIdentityWhoamiCmd(app))

        return cmd
}

func newIdentityCreateCmd(app *App) *cobra.Command {
        var name string
        var kind string
        var use bool
        var userID string

        cmd := &cobra.Command{
                Use:   "create",
                Short: "Create a new identity (actor)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        k, err := store.NormalizeActorKind(kind)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        var parentUserID *string
                        if k == model.ActorKindAgent {
                                if userID == "" {
                                        return writeErr(cmd, errors.New("missing --user (agent identities must belong to a human user)"))
                                }
                                u, ok := db.FindActor(userID)
                                if !ok {
                                        return writeErr(cmd, errNotFound("actor", userID))
                                }
                                if u.Kind != model.ActorKindHuman {
                                        return writeErr(cmd, errors.New("--user must point to a human identity"))
                                }
                                parentUserID = &userID
                        }

                        actor := model.Actor{
                                ID:     s.NextID(db, "act"),
                                Kind:   k,
                                Name:   name,
                                UserID: parentUserID,
                        }
                        db.Actors = append(db.Actors, actor)
                        if use {
                                db.CurrentActorID = actor.ID
                                app.ActorID = actor.ID
                        }
                        if err := s.AppendEvent(actor.ID, "identity.create", actor.ID, map[string]any{"name": name, "kind": kind, "use": use, "ts": time.Now().UTC()}); err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }

                        return writeOut(cmd, app, map[string]any{"data": actor})
                },
        }

        cmd.Flags().StringVar(&name, "name", "", "Display name")
        cmd.Flags().StringVar(&kind, "kind", "human", "Actor kind (human|agent)")
        cmd.Flags().StringVar(&userID, "user", "", "Parent human actor id (required for --kind agent)")
        cmd.Flags().BoolVar(&use, "use", false, "Set as current actor")
        _ = cmd.MarkFlagRequired("name")

        return cmd
}

func newIdentityUseCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "use <actor-id>",
                Short: "Set the current actor for this .clarity database",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        id := args[0]
                        if _, ok := db.FindActor(id); !ok {
                                return writeErr(cmd, errNotFound("actor", id))
                        }
                        db.CurrentActorID = id
                        app.ActorID = id
                        if err := s.AppendEvent(id, "identity.use", id, map[string]any{"actorId": id}); err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{"data": map[string]any{"currentActorId": id}})
                },
        }
        return cmd
}

func newIdentityListCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "list",
                Short: "List identities (actors)",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "currentActorId": db.CurrentActorID,
                                        "actors":         db.Actors,
                                },
                        })
                },
        }
        return cmd
}

func newIdentityWhoamiCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "whoami",
                Short: "Show the current actor",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        id, err := currentActorID(app, db)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        actor, ok := db.FindActor(id)
                        if !ok {
                                return writeErr(cmd, errNotFound("actor", id))
                        }
                        return writeOut(cmd, app, map[string]any{"data": actor})
                },
        }
        return cmd
}
