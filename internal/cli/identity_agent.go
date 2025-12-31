package cli

import (
        "crypto/rand"
        "crypto/sha256"
        "errors"
        "fmt"
        "os"
        "regexp"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

type ensureAgentOpts struct {
        Session string
        Name    string
        UserID  string // human actor id
        Use     bool
}

func newIdentityAgentCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "agent",
                Short: "Agent identities (stable per session)",
        }

        cmd.AddCommand(newIdentityAgentEnsureCmd(app))
        return cmd
}

func newIdentityAgentEnsureCmd(app *App) *cobra.Command {
        var session string
        var name string
        var user string
        var use bool

        cmd := &cobra.Command{
                Use:   "ensure",
                Short: "Ensure an agent identity exists for this session",
                Long: strings.TrimSpace(`
Ensures an agent identity exists for a given session key.

This is intended for autonomous agents (Cursor/Codex/etc) that need:
- a stable identity during a session
- separate identities for separate sessions/agents

If no session is provided, Clarity generates a new session key and creates a new agent identity
(i.e. "new identity per run").
`),
                Example: strings.TrimSpace(`
# Stable identity within a session:
CLARITY_AGENT_SESSION=codex-123 clarity identity agent ensure

# New identity per run (auto session key):
clarity identity agent ensure
`),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        actor, created, sessionKey, err := ensureAgentIdentity(app, db, s, ensureAgentOpts{
                                Session: session,
                                Name:    name,
                                UserID:  user,
                                Use:     use,
                        })
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        return writeOut(cmd, app, map[string]any{
                                "data": actor,
                                "meta": map[string]any{
                                        "created": created,
                                        "session": sessionKey,
                                },
                                "_hints": []string{
                                        "clarity identity whoami",
                                        "clarity items ready",
                                        "clarity agent start <item-id> --session " + sessionKey,
                                },
                        })
                },
        }

        cmd.Flags().StringVar(&session, "session", "", "Agent session key (optional; or set CLARITY_AGENT_SESSION)")
        cmd.Flags().StringVar(&name, "name", envOr("CLARITY_AGENT_NAME", "Agent"), "Agent display name")
        cmd.Flags().StringVar(&user, "user", envOr("CLARITY_AGENT_USER", ""), "Parent human actor id (optional if current actor is set)")
        cmd.Flags().BoolVar(&use, "use", true, "Set as current actor")

        return cmd
}

func ensureAgentIdentity(app *App, db *store.DB, s store.Store, opts ensureAgentOpts) (actor model.Actor, created bool, session string, _ error) {
        session = strings.TrimSpace(opts.Session)
        if session == "" {
                session = strings.TrimSpace(os.Getenv("CLARITY_AGENT_SESSION"))
        }
        if session == "" {
                // Default behavior: create a fresh identity per run if no session is provided.
                // This is "good enough" for most agents and avoids agents having to invent a session key.
                session = autoSessionKey()
        }
        session = normalizeAgentSessionKey(session)

        humanID, err := resolveHumanForAgent(db, app, opts.UserID)
        if err != nil {
                return model.Actor{}, false, "", err
        }

        tag := agentSessionTag(session)
        for i := range db.Actors {
                a := db.Actors[i]
                if a.Kind != model.ActorKindAgent {
                        continue
                }
                if a.UserID == nil || *a.UserID != humanID {
                        continue
                }
                if strings.HasPrefix(a.Name, tag+" ") || strings.HasPrefix(a.Name, legacyAgentSessionTag(tag)+" ") {
                        if opts.Use {
                                db.CurrentActorID = a.ID
                                app.ActorID = a.ID
                                if err := s.AppendEvent(a.ID, "identity.use", a.ID, map[string]any{"actorId": a.ID}); err != nil {
                                        return model.Actor{}, false, "", err
                                }
                                if err := s.Save(db); err != nil {
                                        return model.Actor{}, false, "", err
                                }
                        }
                        return a, false, tag, nil
                }
        }

        display := strings.TrimSpace(opts.Name)
        if display == "" {
                display = "Agent"
        }

        uid := humanID
        a := model.Actor{
                ID:     s.NextID(db, "act"),
                Kind:   model.ActorKindAgent,
                Name:   fmt.Sprintf("%s %s", tag, display),
                UserID: &uid,
        }
        db.Actors = append(db.Actors, a)
        if opts.Use {
                db.CurrentActorID = a.ID
                app.ActorID = a.ID
        }
        if err := s.AppendEvent(a.ID, "identity.create", a.ID, map[string]any{
                "name":    a.Name,
                "kind":    "agent",
                "use":     opts.Use,
                "userId":  humanID,
                "session": session,
                "ts":      time.Now().UTC(),
        }); err != nil {
                return model.Actor{}, false, "", err
        }
        if err := s.Save(db); err != nil {
                return model.Actor{}, false, "", err
        }
        return a, true, tag, nil
}

func resolveHumanForAgent(db *store.DB, app *App, userFlag string) (string, error) {
        userFlag = strings.TrimSpace(userFlag)
        if userFlag != "" {
                u, ok := db.FindActor(userFlag)
                if !ok {
                        return "", errNotFound("actor", userFlag)
                }
                if u.Kind != model.ActorKindHuman {
                        return "", errors.New("--user must point to a human identity")
                }
                return u.ID, nil
        }

        // Fall back to current actor => resolve owning human.
        cur, err := currentActorID(app, db)
        if err != nil {
                return "", errors.New("missing --user (or set a current actor with `clarity identity use <human-actor-id>`)")
        }
        if humanID, ok := db.HumanUserIDForActor(cur); ok {
                return humanID, nil
        }
        return "", errors.New("unable to resolve human user for current actor; pass --user <human-actor-id>")
}

func agentSessionTag(session string) string {
        // Stored in actor.Name so we can look up the session identity without changing schema.
        return strings.TrimSpace(session)
}

func legacyAgentSessionTag(session string) string {
        return "[agent-session:" + strings.TrimSpace(session) + "]"
}

var sessionDateSuffixRe = regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9_-]*)-\d{4}-\d{2}-\d{2}$`)

func normalizeAgentSessionKey(session string) string {
        session = strings.TrimSpace(session)
        if session == "" {
                return session
        }
        m := sessionDateSuffixRe.FindStringSubmatch(session)
        if m == nil {
                return session
        }
        prefix := strings.TrimSpace(m[1])
        if prefix == "" {
                return session
        }
        // Deterministic short suffix so the same input session always maps to the same key.
        // This preserves the "stable identity per session" property while removing the confusing date.
        return prefix + "-" + stableLettersFromString(session, 3)
}

func autoSessionKey() string {
        // Intentionally avoids dates/timestamps to reduce confusion in generated agent session IDs.
        return randomLetters(3)
}

func randomLetters(n int) string {
        const letters = "abcdefghijklmnopqrstuvwxyz"
        if n <= 0 {
                return ""
        }
        b := make([]byte, n)
        if _, err := rand.Read(b); err == nil {
                for i := range b {
                        b[i] = letters[int(b[i])%len(letters)]
                }
                return string(b)
        }

        // Extremely rare fallback: derive letters deterministically from host + pid (no timestamps).
        host, _ := os.Hostname()
        sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", os.Getpid(), host)))
        out := make([]byte, n)
        for i := range out {
                out[i] = letters[int(sum[i])%len(letters)]
        }
        return string(out)
}

func stableLettersFromString(s string, n int) string {
        const letters = "abcdefghijklmnopqrstuvwxyz"
        if n <= 0 {
                return ""
        }
        sum := sha256.Sum256([]byte(s))
        out := make([]byte, n)
        for i := range out {
                out[i] = letters[int(sum[i])%len(letters)]
        }
        return string(out)
}
