//go:build integration

package cli

import (
        "encoding/json"
        "testing"
)

func TestCLIIntegrationSmoke(t *testing.T) {
        dir := t.TempDir()

        mustRun := func(args ...string) map[string]any {
                t.Helper()
                stdout, stderr, err := runCLI(t, args)
                if err != nil {
                        t.Fatalf("command failed: clarity %v\nerr: %v\nstderr:\n%s\nstdout:\n%s", args, err, string(stderr), string(stdout))
                }
                var env map[string]any
                if err := json.Unmarshal(stdout, &env); err != nil {
                        t.Fatalf("unmarshal stdout as json envelope: %v\nstdout:\n%s\nargs: %v", err, string(stdout), args)
                }
                if _, ok := env["data"]; !ok {
                        t.Fatalf("expected JSON envelope to contain data key; got: %v\nstdout:\n%s", env, string(stdout))
                }
                return env
        }

        // Init isolated store (no ~/.clarity workspace config should be touched when using --dir).
        mustRun("--dir", dir, "init")

        // Identity + project setup.
        ident := mustRun("--dir", dir, "identity", "create", "--name", "Integration Human", "--kind", "human", "--use")
        humanID, _ := ident["data"].(map[string]any)["id"].(string)
        if humanID == "" {
                t.Fatalf("expected identity create to return actor id; got: %#v", ident["data"])
        }

        proj := mustRun("--dir", dir, "--actor", humanID, "projects", "create", "--name", "Integration Project", "--use")
        projectID, _ := proj["data"].(map[string]any)["id"].(string)
        if projectID == "" {
                t.Fatalf("expected projects create to return project id; got: %#v", proj["data"])
        }

        // Create a few items (this will auto-create a default outline in the project).
        a := mustRun("--dir", dir, "--actor", humanID, "items", "create", "--project", projectID, "--title", "Item A", "--description", "A desc")
        aID, _ := a["data"].(map[string]any)["id"].(string)
        if aID == "" {
                t.Fatalf("expected items create to return item id; got: %#v", a["data"])
        }
        b := mustRun("--dir", dir, "--actor", humanID, "items", "create", "--project", projectID, "--title", "Item B")
        bID, _ := b["data"].(map[string]any)["id"].(string)
        if bID == "" {
                t.Fatalf("expected items create to return item id; got: %#v", b["data"])
        }
        c := mustRun("--dir", dir, "--actor", humanID, "items", "create", "--project", projectID, "--title", "Item C")
        cID, _ := c["data"].(map[string]any)["id"].(string)
        if cID == "" {
                t.Fatalf("expected items create to return item id; got: %#v", c["data"])
        }

        // Exercise item updates.
        mustRun("--dir", dir, "--actor", humanID, "items", "set-status", aID, "--status", "doing")
        mustRun("--dir", dir, "--actor", humanID, "items", "set-priority", aID)
        mustRun("--dir", dir, "--actor", humanID, "items", "set-description", aID, "--description", "Updated description")

        // Deps: A blocked by B, B blocked by C. Verify tree + cycles detection.
        mustRun("--dir", dir, "--actor", humanID, "deps", "add", aID, "--blocks", bID)
        mustRun("--dir", dir, "--actor", humanID, "deps", "add", bID, "--blocks", cID)
        mustRun("--dir", dir, "--actor", humanID, "deps", "list", aID)
        mustRun("--dir", dir, "--actor", humanID, "deps", "tree", aID)
        cycles0 := mustRun("--dir", dir, "--actor", humanID, "deps", "cycles")
        if xs, ok := cycles0["data"].([]any); ok && len(xs) != 0 {
                t.Fatalf("expected no cycles initially; got: %#v", cycles0["data"])
        }
        mustRun("--dir", dir, "--actor", humanID, "deps", "add", cID, "--blocks", aID) // introduce cycle
        cycles1 := mustRun("--dir", dir, "--actor", humanID, "deps", "cycles")
        if xs, ok := cycles1["data"].([]any); !ok || len(xs) == 0 {
                t.Fatalf("expected cycles after creating one; got: %#v", cycles1["data"])
        }

        // Ready list (should not include blocked items; should include at least one unblocked item if present).
        ready := mustRun("--dir", dir, "--actor", humanID, "items", "ready", "--include-assigned")
        if xs, ok := ready["data"].([]any); !ok {
                t.Fatalf("expected items ready data to be a list; got: %#v", ready["data"])
        } else if len(xs) != 0 {
                // Since we introduced a cycle, all items are now blocked; if we still got items, something is off.
                t.Fatalf("expected no ready items after cycle; got: %#v", ready["data"])
        }

        // Comments + worklog (exercise author filtering + pagination envelope).
        mustRun("--dir", dir, "--actor", humanID, "comments", "add", aID, "--body", "Comment 1")
        comments := mustRun("--dir", dir, "--actor", humanID, "comments", "list", aID, "--limit", "0")
        if xs, ok := comments["data"].([]any); !ok || len(xs) == 0 {
                t.Fatalf("expected comments list to return at least one comment; got: %#v", comments["data"])
        }

        mustRun("--dir", dir, "--actor", humanID, "worklog", "add", aID, "--body", "Worklog 1")
        worklog := mustRun("--dir", dir, "--actor", humanID, "worklog", "list", aID, "--limit", "0")
        if xs, ok := worklog["data"].([]any); !ok || len(xs) == 0 {
                t.Fatalf("expected worklog list to return at least one entry; got: %#v", worklog["data"])
        }

        // Show + events.
        mustRun("--dir", dir, "--actor", humanID, "items", "show", aID)
        mustRun("--dir", dir, "--actor", humanID, "items", "events", aID, "--limit", "0")

        // Agent workflow: ensure agent identity (session-scoped) and claim item.
        agentStart := mustRun("--dir", dir, "--actor", humanID, "agent", "start", aID, "--session", "integration-session", "--name", "Integration Agent")
        data, _ := agentStart["data"].(map[string]any)
        actor, _ := data["actor"].(map[string]any)
        agentID, _ := actor["id"].(string)
        if agentID == "" {
                t.Fatalf("expected agent start to return agent actor id; got: %#v", agentStart["data"])
        }
        who := mustRun("--dir", dir, "--actor", agentID, "identity", "whoami")
        if id, _ := who["data"].(map[string]any)["id"].(string); id != agentID {
                t.Fatalf("expected whoami to be agent %q; got: %#v", agentID, who["data"])
        }
}
