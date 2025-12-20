package cli

import (
        "errors"
        "strings"

        "github.com/spf13/cobra"
)

func newAgentCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "agent",
                Short: "Agent workflow helpers (session identity + claiming items)",
                Long: strings.TrimSpace(`
High-level agent workflow:
- Pick work: clarity items ready
- Start work (ensures identity + claims item): clarity agent start <item-id>

Identity pattern:
- Stable identity per session: set CLARITY_AGENT_SESSION (recommended)
- New identity per run: omit session (Clarity will generate one)
`),
        }

        cmd.AddCommand(newAgentStartCmd(app))
        return cmd
}

func newAgentStartCmd(app *App) *cobra.Command {
        var session string
        var name string
        var user string
        var takeAssigned bool

        cmd := &cobra.Command{
                Use:   "start <item-id>",
                Short: "Ensure agent identity for this session and claim the item",
                Long: strings.TrimSpace(`
Creates (or reuses) an agent identity for the given session key, sets it as the current actor,
and assigns the item to that agent (claiming ownership when allowed).

This is designed for autonomous agents: stable identity per session + explicit ownership attribution.

If no session is provided, Clarity generates a new session key and creates a new agent identity
(i.e. "new identity per run").
`),
                Example: strings.TrimSpace(`
# Stable identity for the duration of a session:
CLARITY_AGENT_SESSION=codex-123 clarity agent start <item-id>

# New identity per run:
clarity agent start <item-id>

# Explicitly take an already-assigned item:
clarity agent start <item-id> --take-assigned
`),
                Args: cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        itemID := strings.TrimSpace(args[0])
                        if itemID == "" {
                                return writeErr(cmd, errors.New("missing <item-id>"))
                        }

                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        actor, created, err := ensureAgentIdentity(app, db, s, ensureAgentOpts{
                                Session: session,
                                Name:    name,
                                UserID:  user,
                                Use:     true,
                        })
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        updated, err := claimItemAsCurrentActor(app, db, s, itemID, claimOpts{TakeAssigned: takeAssigned})
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": map[string]any{
                                        "actor": actor,
                                        "item":  updated,
                                },
                                "meta": map[string]any{
                                        "agentCreated": created,
                                },
                                "_hints": []string{
                                        "clarity identity whoami",
                                        "clarity items show " + itemID,
                                        "clarity worklog add " + itemID + " --body \"...\"",
                                },
                        })
                },
        }

        cmd.Flags().StringVar(&session, "session", envOr("CLARITY_AGENT_SESSION", ""), "Agent session key (optional; or set CLARITY_AGENT_SESSION)")
        cmd.Flags().StringVar(&name, "name", envOr("CLARITY_AGENT_NAME", "Agent"), "Agent display name")
        cmd.Flags().StringVar(&user, "user", envOr("CLARITY_AGENT_USER", ""), "Parent human actor id (optional if current actor is set)")
        cmd.Flags().BoolVar(&takeAssigned, "take-assigned", false, "Take the item even if it's already assigned to another actor")

        return cmd
}
