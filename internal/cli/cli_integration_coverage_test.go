//go:build integration

package cli

import (
        "bytes"
        "encoding/json"
        "fmt"
        "os"
        "sort"
        "strings"
        "testing"

        "github.com/spf13/cobra"
        "github.com/spf13/pflag"
)

type expectKind int

const (
        expectJSONEnvelope expectKind = iota
        expectEDNEnvelope
        expectRawText
        expectError
)

type invocation struct {
        name   string
        args   []string
        env    map[string]string
        expect expectKind
        // cmdPath is the Cobra command path (without the root "clarity"), e.g. "items create".
        cmdPath string
        // markEnvFlags counts env-var based "options" as covered flags (e.g. CLARITY_WORKSPACE => workspace).
        markEnvFlags []string
}

type runResult struct {
        stdout []byte
        stderr []byte
        env    map[string]any
}

func TestCLIIntegration_CommandAndFlagCoverage(t *testing.T) {
        // Keep global config writes contained.
        t.Setenv("CLARITY_CONFIG_DIR", t.TempDir())

        // Seed an isolated store (dir mode) for most commands.
        dir := t.TempDir()

        // Helper: run a command with optional env, and validate output.
        coveredCmds := map[string]bool{}
        coveredFlags := map[string]bool{}                      // long flag names only (no leading --), global
        coveredLocalFlagsByCmd := map[string]map[string]bool{} // cmdPath -> flagName -> covered (flags used when invoking that command)

        run := func(t *testing.T, inv invocation) runResult {
                t.Helper()

                if strings.TrimSpace(inv.cmdPath) != "" {
                        coveredCmds[inv.cmdPath] = true
                        if _, ok := coveredLocalFlagsByCmd[inv.cmdPath]; !ok {
                                coveredLocalFlagsByCmd[inv.cmdPath] = map[string]bool{}
                        }
                }

                // Reset commonly-used env options between invocations so one test case doesn't
                // accidentally change the output format or store resolution of later cases.
                // (t.Setenv persists for the whole test once set.)
                for _, k := range []string{
                        "CLARITY_DIR",
                        "CLARITY_WORKSPACE",
                        "CLARITY_ACTOR",
                        "CLARITY_FORMAT",
                        "CLARITY_AGENT_SESSION",
                        "CLARITY_AGENT_NAME",
                        "CLARITY_AGENT_USER",
                        "CLARITY_ASSIGN_GRACE_SECONDS",
                } {
                        if inv.env == nil {
                                t.Setenv(k, "")
                                continue
                        }
                        if _, ok := inv.env[k]; !ok {
                                t.Setenv(k, "")
                        }
                }

                for k, v := range inv.env {
                        t.Setenv(k, v)
                }
                used := flagsFromArgs(inv.args)
                for _, f := range used {
                        coveredFlags[f] = true
                        if strings.TrimSpace(inv.cmdPath) != "" {
                                coveredLocalFlagsByCmd[inv.cmdPath][f] = true
                        }
                }
                for _, f := range inv.markEnvFlags {
                        coveredFlags[f] = true
                        if strings.TrimSpace(inv.cmdPath) != "" {
                                coveredLocalFlagsByCmd[inv.cmdPath][f] = true
                        }
                }

                stdout, stderr, err := runCLI(t, inv.args)
                switch inv.expect {
                case expectError:
                        if err == nil {
                                t.Fatalf("expected error but command succeeded: clarity %v\nstdout:\n%s\nstderr:\n%s", inv.args, string(stdout), string(stderr))
                        }
                        if len(bytes.TrimSpace(stderr)) == 0 {
                                t.Fatalf("expected stderr for failing command: clarity %v\nstdout:\n%s", inv.args, string(stdout))
                        }
                        return runResult{stdout: stdout, stderr: stderr}
                case expectRawText:
                        if err != nil {
                                t.Fatalf("expected success: clarity %v\nerr: %v\nstderr:\n%s\nstdout:\n%s", inv.args, err, string(stderr), string(stdout))
                        }
                        if len(bytes.TrimSpace(stdout)) == 0 {
                                t.Fatalf("expected raw text on stdout: clarity %v\nstderr:\n%s", inv.args, string(stderr))
                        }
                        return runResult{stdout: stdout, stderr: stderr}
                case expectEDNEnvelope:
                        if err != nil {
                                t.Fatalf("expected success: clarity %v\nerr: %v\nstderr:\n%s\nstdout:\n%s", inv.args, err, string(stderr), string(stdout))
                        }
                        assertEDNEnvelope(t, stdout, inv.args)
                        return runResult{stdout: stdout, stderr: stderr}
                case expectJSONEnvelope:
                        if err != nil {
                                t.Fatalf("expected success: clarity %v\nerr: %v\nstderr:\n%s\nstdout:\n%s", inv.args, err, string(stderr), string(stdout))
                        }
                        env := assertJSONEnvelope(t, stdout, inv.args)
                        return runResult{stdout: stdout, stderr: stderr, env: env}
                default:
                        t.Fatalf("unknown expect kind")
                        return runResult{}
                }
        }

        // --- Seed base entities in dir mode ---
        initEnv := run(t, invocation{name: "init", cmdPath: "init", args: []string{"--dir", dir, "init"}, expect: expectJSONEnvelope}).env
        assertDataMapHasKeys(t, initEnv, "dir", "sqlitePath")

        humanID := mustActorID(t, run(t, invocation{
                name:    "identity create (human, --use)",
                cmdPath: "identity create",
                args:    []string{"--dir", dir, "identity", "create", "--name", "Integration Human", "--kind", "human", "--use"},
                expect:  expectJSONEnvelope,
        }).env)

        // Make another human for assignment/permission edge cases.
        human2ID := mustActorID(t, run(t, invocation{
                name:    "identity create (second human)",
                cmdPath: "identity create",
                args:    []string{"--dir", dir, "--actor", humanID, "identity", "create", "--name", "Second Human", "--kind", "human"},
                expect:  expectJSONEnvelope,
        }).env)

        projectID := mustID(t, run(t, invocation{
                name:    "projects create (--use)",
                cmdPath: "projects create",
                args:    []string{"--dir", dir, "--actor", humanID, "projects", "create", "--name", "Integration Project", "--use"},
                expect:  expectJSONEnvelope,
        }).env)

        // Create a second outline so we can cover --outline selection and move-outline.
        out1 := mustID(t, run(t, invocation{
                name:    "outlines create (named)",
                cmdPath: "outlines create",
                args:    []string{"--dir", dir, "--actor", humanID, "outlines", "create", "--project", projectID, "--name", "Outline A"},
                expect:  expectJSONEnvelope,
        }).env)
        out2 := mustID(t, run(t, invocation{
                name:    "outlines create (second)",
                cmdPath: "outlines create",
                args:    []string{"--dir", dir, "--actor", humanID, "outlines", "create", "--project", projectID, "--name", "Outline B"},
                expect:  expectJSONEnvelope,
        }).env)

        // Items: create a few, exercising create flags.
        itemA := mustID(t, run(t, invocation{
                name:    "items create (with description + filed-from)",
                cmdPath: "items create",
                args: []string{
                        "--dir", dir, "--actor", humanID,
                        "items", "create",
                        "--project", projectID,
                        "--outline", out1,
                        "--title", "Item A",
                        "--description", "A desc",
                        "--filed-from", "somewhere",
                },
                expect: expectJSONEnvelope,
        }).env)
        itemB := mustID(t, run(t, invocation{
                name:    "items create (assign via --assign)",
                cmdPath: "items create",
                args: []string{
                        "--dir", dir, "--actor", humanID,
                        "items", "create",
                        "--project", projectID,
                        "--outline", out1,
                        "--title", "Item B",
                        "--assign", humanID,
                },
                expect: expectJSONEnvelope,
        }).env)
        itemC := mustID(t, run(t, invocation{
                name:    "items create (owner override)",
                cmdPath: "items create",
                args: []string{
                        "--dir", dir, "--actor", humanID,
                        "items", "create",
                        "--project", projectID,
                        "--outline", out1,
                        "--title", "Item C",
                        "--owner", human2ID,
                },
                expect: expectJSONEnvelope,
        }).env)

        // Parent nesting via --parent at create time.
        child := mustID(t, run(t, invocation{
                name:    "items create (with --parent)",
                cmdPath: "items create",
                args: []string{
                        "--dir", dir, "--actor", humanID,
                        "items", "create",
                        "--project", projectID,
                        "--outline", out1,
                        "--parent", itemA,
                        "--title", "Child of A",
                },
                expect: expectJSONEnvelope,
        }).env)
        _ = child

        // --- Items list/show/events, with output-contract checks on meta and hints where applicable ---
        showEnv := run(t, invocation{name: "items show", cmdPath: "items show", args: []string{"--dir", dir, "--actor", humanID, "items", "show", itemA}, expect: expectJSONEnvelope}).env
        assertItemsShowEnvelope(t, showEnv)

        evEnv := run(t, invocation{name: "items events (--limit 0)", cmdPath: "items events", args: []string{"--dir", dir, "--actor", humanID, "items", "events", itemA, "--limit", "0"}, expect: expectJSONEnvelope}).env
        assertDataIsSlice(t, evEnv)

        // Update commands + their flags.
        run(t, invocation{name: "items set-title", cmdPath: "items set-title", args: []string{"--dir", dir, "--actor", humanID, "items", "set-title", itemA, "--title", "Item A (renamed)"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-description", cmdPath: "items set-description", args: []string{"--dir", dir, "--actor", humanID, "items", "set-description", itemA, "--description", "Updated description"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-status", cmdPath: "items set-status", args: []string{"--dir", dir, "--actor", humanID, "items", "set-status", itemA, "--status", "doing"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-status --note", cmdPath: "items set-status", args: []string{"--dir", dir, "--actor", humanID, "items", "set-status", itemA, "--status", "todo", "--note", "context"}, expect: expectJSONEnvelope})
        // Negative: invalid status id/label for outline.
        run(t, invocation{name: "items set-status (invalid)", cmdPath: "items set-status", args: []string{"--dir", dir, "--actor", humanID, "items", "set-status", itemA, "--status", "does-not-exist"}, expect: expectError})

        // set-priority: toggle, then explicit --on/--off.
        run(t, invocation{name: "items set-priority (toggle)", cmdPath: "items set-priority", args: []string{"--dir", dir, "--actor", humanID, "items", "set-priority", itemA}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-priority --on", cmdPath: "items set-priority", args: []string{"--dir", dir, "--actor", humanID, "items", "set-priority", itemA, "--on"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-priority --off", cmdPath: "items set-priority", args: []string{"--dir", dir, "--actor", humanID, "items", "set-priority", itemA, "--off"}, expect: expectJSONEnvelope})

        // set-on-hold: toggle, then explicit --on/--off.
        run(t, invocation{name: "items set-on-hold (toggle)", cmdPath: "items set-on-hold", args: []string{"--dir", dir, "--actor", humanID, "items", "set-on-hold", itemA}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-on-hold --on", cmdPath: "items set-on-hold", args: []string{"--dir", dir, "--actor", humanID, "items", "set-on-hold", itemA, "--on"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-on-hold --off", cmdPath: "items set-on-hold", args: []string{"--dir", dir, "--actor", humanID, "items", "set-on-hold", itemA, "--off"}, expect: expectJSONEnvelope})

        // set-due/schedule: set + clear.
        run(t, invocation{name: "items set-due --at", cmdPath: "items set-due", args: []string{"--dir", dir, "--actor", humanID, "items", "set-due", itemA, "--at", "2025-12-31"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-due --clear", cmdPath: "items set-due", args: []string{"--dir", dir, "--actor", humanID, "items", "set-due", itemA, "--clear"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-schedule --at", cmdPath: "items set-schedule", args: []string{"--dir", dir, "--actor", humanID, "items", "set-schedule", itemA, "--at", "2025-12-30 09:00"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-schedule --clear", cmdPath: "items set-schedule", args: []string{"--dir", dir, "--actor", humanID, "items", "set-schedule", itemA, "--clear"}, expect: expectJSONEnvelope})

        // set-assign: use --assignee, alias --to, and --clear.
        run(t, invocation{name: "items set-assign --assignee", cmdPath: "items set-assign", args: []string{"--dir", dir, "--actor", humanID, "items", "set-assign", itemB, "--assignee", human2ID}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-assign --to (alias)", cmdPath: "items set-assign", args: []string{"--dir", dir, "--actor", human2ID, "items", "set-assign", itemB, "--to", human2ID}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-assign --clear", cmdPath: "items set-assign", args: []string{"--dir", dir, "--actor", human2ID, "items", "set-assign", itemB, "--clear"}, expect: expectJSONEnvelope})

        // tags: add/remove/set (repeatable).
        run(t, invocation{name: "items tags add", cmdPath: "items tags add", args: []string{"--dir", dir, "--actor", humanID, "items", "tags", "add", itemA, "--tag", "t1"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items tags remove", cmdPath: "items tags remove", args: []string{"--dir", dir, "--actor", humanID, "items", "tags", "remove", itemA, "--tag", "t1"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items tags set", cmdPath: "items tags set", args: []string{"--dir", dir, "--actor", humanID, "items", "tags", "set", itemA, "--tag", "t1", "--tag", "t2"}, expect: expectJSONEnvelope})

        // archive/unarchive.
        run(t, invocation{name: "items archive", cmdPath: "items archive", args: []string{"--dir", dir, "--actor", humanID, "items", "archive", itemA}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items archive --unarchive", cmdPath: "items archive", args: []string{"--dir", dir, "--actor", humanID, "items", "archive", itemA, "--unarchive"}, expect: expectJSONEnvelope})

        // items list filters.
        run(t, invocation{name: "items list (project)", cmdPath: "items list", args: []string{"--dir", dir, "--actor", humanID, "items", "list", "--project", projectID}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items list (outline)", cmdPath: "items list", args: []string{"--dir", dir, "--actor", humanID, "items", "list", "--outline", out1}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items list (mine)", cmdPath: "items list", args: []string{"--dir", dir, "--actor", humanID, "items", "list", "--mine"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items list (status)", cmdPath: "items list", args: []string{"--dir", dir, "--actor", humanID, "items", "list", "--status", "doing"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items list (include-archived)", cmdPath: "items list", args: []string{"--dir", dir, "--actor", humanID, "items", "list", "--include-archived"}, expect: expectJSONEnvelope})

        // ready (with include-assigned)
        run(t, invocation{name: "items ready --include-assigned", cmdPath: "items ready", args: []string{"--dir", dir, "--actor", humanID, "items", "ready", "--include-assigned"}, expect: expectJSONEnvelope})
        // ready (include on-hold items)
        run(t, invocation{name: "items ready --include-on-hold", cmdPath: "items ready", args: []string{"--dir", dir, "--actor", humanID, "items", "ready", "--include-on-hold"}, expect: expectJSONEnvelope})

        // move + set-parent + move-outline
        run(t, invocation{name: "items move --before", cmdPath: "items move", args: []string{"--dir", dir, "--actor", humanID, "items", "move", itemB, "--before", itemA}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items move --after", cmdPath: "items move", args: []string{"--dir", dir, "--actor", humanID, "items", "move", itemB, "--after", itemA}, expect: expectJSONEnvelope})
        // Negative: invalid move args (before+after).
        run(t, invocation{name: "items move (before+after invalid)", cmdPath: "items move", args: []string{"--dir", dir, "--actor", humanID, "items", "move", itemB, "--before", itemA, "--after", itemA}, expect: expectError})
        run(t, invocation{name: "items set-parent (root -> itemA)", cmdPath: "items set-parent", args: []string{"--dir", dir, "--actor", humanID, "items", "set-parent", itemB, "--parent", itemA}, expect: expectJSONEnvelope})
        // itemC is owned by human2 (created with --owner), so reparent operations must be run as human2.
        run(t, invocation{name: "items set-parent (place before in dest)", cmdPath: "items set-parent", args: []string{"--dir", dir, "--actor", human2ID, "items", "set-parent", itemC, "--parent", itemA, "--before", itemB}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-parent (place after in dest)", cmdPath: "items set-parent", args: []string{"--dir", dir, "--actor", human2ID, "items", "set-parent", itemC, "--parent", itemA, "--after", itemB}, expect: expectJSONEnvelope})
        // Negative: disallow parent cycles (set A's parent to its child).
        run(t, invocation{name: "items set-parent (cycle invalid)", cmdPath: "items set-parent", args: []string{"--dir", dir, "--actor", humanID, "items", "set-parent", itemA, "--parent", child}, expect: expectError})

        // move-outline requires --to, optionally --set-status.
        run(t, invocation{name: "items move-outline --to", cmdPath: "items move-outline", args: []string{"--dir", dir, "--actor", humanID, "items", "move-outline", itemB, "--to", out2}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items move-outline --set-status", cmdPath: "items move-outline", args: []string{"--dir", dir, "--actor", humanID, "items", "move-outline", itemB, "--to", out1, "--set-status", "todo"}, expect: expectJSONEnvelope})

        // deps: add blocks + related, list, tree, cycles + an error path.
        run(t, invocation{name: "deps add --blocks", cmdPath: "deps add", args: []string{"--dir", dir, "--actor", humanID, "deps", "add", itemA, "--blocks", itemB}, expect: expectJSONEnvelope})
        run(t, invocation{name: "deps add --related", cmdPath: "deps add", args: []string{"--dir", dir, "--actor", humanID, "deps", "add", itemA, "--related", itemC}, expect: expectJSONEnvelope})
        run(t, invocation{name: "deps add (missing flags)", cmdPath: "deps add", args: []string{"--dir", dir, "--actor", humanID, "deps", "add", itemA}, expect: expectError})
        run(t, invocation{name: "deps add (blocks+related invalid)", cmdPath: "deps add", args: []string{"--dir", dir, "--actor", humanID, "deps", "add", itemA, "--blocks", itemB, "--related", itemC}, expect: expectError})
        run(t, invocation{name: "deps list (all)", cmdPath: "deps list", args: []string{"--dir", dir, "--actor", humanID, "deps", "list"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "deps list (for item)", cmdPath: "deps list", args: []string{"--dir", dir, "--actor", humanID, "deps", "list", itemA}, expect: expectJSONEnvelope})
        run(t, invocation{name: "deps tree", cmdPath: "deps tree", args: []string{"--dir", dir, "--actor", humanID, "deps", "tree", itemA}, expect: expectJSONEnvelope})
        run(t, invocation{name: "deps cycles", cmdPath: "deps cycles", args: []string{"--dir", dir, "--actor", humanID, "deps", "cycles"}, expect: expectJSONEnvelope})

        // comments: add + list pagination.
        run(t, invocation{name: "comments add", cmdPath: "comments add", args: []string{"--dir", dir, "--actor", humanID, "comments", "add", itemA, "--body", "Comment 1"}, expect: expectJSONEnvelope})
        assertPaginatedListMeta(t, run(t, invocation{name: "comments list (limit/offset)", cmdPath: "comments list", args: []string{"--dir", dir, "--actor", humanID, "comments", "list", itemA, "--limit", "1", "--offset", "0"}, expect: expectJSONEnvelope}).env)
        run(t, invocation{name: "comments list (all)", cmdPath: "comments list", args: []string{"--dir", dir, "--actor", humanID, "comments", "list", itemA, "--limit", "0"}, expect: expectJSONEnvelope})

        // worklog: add + list pagination.
        run(t, invocation{name: "worklog add", cmdPath: "worklog add", args: []string{"--dir", dir, "--actor", humanID, "worklog", "add", itemA, "--body", "Worklog 1"}, expect: expectJSONEnvelope})
        assertPaginatedListMeta(t, run(t, invocation{name: "worklog list (limit/offset)", cmdPath: "worklog list", args: []string{"--dir", dir, "--actor", humanID, "worklog", "list", itemA, "--limit", "1", "--offset", "0"}, expect: expectJSONEnvelope}).env)
        run(t, invocation{name: "worklog list (all)", cmdPath: "worklog list", args: []string{"--dir", dir, "--actor", humanID, "worklog", "list", itemA, "--limit", "0"}, expect: expectJSONEnvelope})

        // events: list with limit.
        run(t, invocation{name: "events list --limit", cmdPath: "events list", args: []string{"--dir", dir, "--actor", humanID, "events", "list", "--limit", "0"}, expect: expectJSONEnvelope})

        // publish: derived Markdown export.
        pubDir := t.TempDir()
        run(t, invocation{name: "publish item (--to, flags)", cmdPath: "publish item", args: []string{"--dir", dir, "--actor", humanID, "publish", "item", itemA, "--to", pubDir, "--include-worklog", "--overwrite=false"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "publish outline (--to, flags)", cmdPath: "publish outline", args: []string{"--dir", dir, "--actor", humanID, "publish", "outline", out1, "--to", pubDir, "--include-archived"}, expect: expectJSONEnvelope})

        // status: should produce envelope and be stable.
        run(t, invocation{name: "status", cmdPath: "status", args: []string{"--dir", dir, "--actor", humanID, "status"}, expect: expectJSONEnvelope})

        // outlines: list/show/archive/unarchive + status defs.
        run(t, invocation{name: "outlines list", cmdPath: "outlines list", args: []string{"--dir", dir, "--actor", humanID, "outlines", "list"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "outlines list --project", cmdPath: "outlines list", args: []string{"--dir", dir, "--actor", humanID, "outlines", "list", "--project", projectID}, expect: expectJSONEnvelope})
        run(t, invocation{name: "outlines show", cmdPath: "outlines show", args: []string{"--dir", dir, "--actor", humanID, "outlines", "show", out1}, expect: expectJSONEnvelope})
        run(t, invocation{name: "outlines archive", cmdPath: "outlines archive", args: []string{"--dir", dir, "--actor", humanID, "outlines", "archive", out2}, expect: expectJSONEnvelope})
        run(t, invocation{name: "outlines archive --unarchive", cmdPath: "outlines archive", args: []string{"--dir", dir, "--actor", humanID, "outlines", "archive", out2, "--unarchive"}, expect: expectJSONEnvelope})

        // Outline statuses: list/add/update/remove/reorder.
        run(t, invocation{name: "outlines status list", cmdPath: "outlines status list", args: []string{"--dir", dir, "--actor", humanID, "outlines", "status", "list", out1}, expect: expectJSONEnvelope})
        // Add a new status to out1 and capture its label for reorder/update/remove.
        run(t, invocation{name: "outlines status add --label --end", cmdPath: "outlines status add", args: []string{"--dir", dir, "--actor", humanID, "outlines", "status", "add", out1, "--label", "Blocked", "--end"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "outlines status add --require-note", cmdPath: "outlines status add", args: []string{"--dir", dir, "--actor", humanID, "outlines", "status", "add", out1, "--label", "NeedsNote", "--require-note"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "outlines status update --label --not-end", cmdPath: "outlines status update", args: []string{"--dir", dir, "--actor", humanID, "outlines", "status", "update", out1, "Blocked", "--label", "Blocked2", "--not-end"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "outlines status update --end", cmdPath: "outlines status update", args: []string{"--dir", dir, "--actor", humanID, "outlines", "status", "update", out1, "Blocked2", "--end"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "outlines status update --no-require-note", cmdPath: "outlines status update", args: []string{"--dir", dir, "--actor", humanID, "outlines", "status", "update", out1, "NeedsNote", "--no-require-note"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "outlines status update --require-note", cmdPath: "outlines status update", args: []string{"--dir", dir, "--actor", humanID, "outlines", "status", "update", out1, "NeedsNote", "--require-note"}, expect: expectJSONEnvelope})
        // Reorder requires all labels exactly once. We can read them from `outlines show`.
        statusLabels := mustStatusLabels(t, run(t, invocation{name: "outlines show (for reorder)", cmdPath: "outlines show", args: []string{"--dir", dir, "--actor", humanID, "outlines", "show", out1}, expect: expectJSONEnvelope}).env)
        rev := append([]string{}, statusLabels...)
        for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
                rev[i], rev[j] = rev[j], rev[i]
        }
        args := []string{"--dir", dir, "--actor", humanID, "outlines", "status", "reorder", out1}
        for _, l := range rev {
                args = append(args, "--label", l)
        }
        run(t, invocation{name: "outlines status reorder --label...", cmdPath: "outlines status reorder", args: args, expect: expectJSONEnvelope})
        // Remove the newly-added status (now labeled Blocked2). Ensure it's not in use.
        run(t, invocation{name: "outlines status remove", cmdPath: "outlines status remove", args: []string{"--dir", dir, "--actor", humanID, "outlines", "status", "remove", out1, "Blocked2"}, expect: expectJSONEnvelope})
        // Negative: cannot remove a status that is in use by an item.
        run(t, invocation{name: "outlines status add (in-use)", cmdPath: "outlines status add", args: []string{"--dir", dir, "--actor", humanID, "outlines", "status", "add", out1, "--label", "InUse"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items set-status (to InUse)", cmdPath: "items set-status", args: []string{"--dir", dir, "--actor", humanID, "items", "set-status", itemA, "--status", "InUse"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "outlines status remove (blocked by in-use)", cmdPath: "outlines status remove", args: []string{"--dir", dir, "--actor", humanID, "outlines", "status", "remove", out1, "InUse"}, expect: expectError})

        // projects: list/current/use/archive/unarchive
        run(t, invocation{name: "projects list", cmdPath: "projects list", args: []string{"--dir", dir, "--actor", humanID, "projects", "list"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "projects current", cmdPath: "projects current", args: []string{"--dir", dir, "--actor", humanID, "projects", "current"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "projects use", cmdPath: "projects use", args: []string{"--dir", dir, "--actor", humanID, "projects", "use", projectID}, expect: expectJSONEnvelope})
        run(t, invocation{name: "projects archive", cmdPath: "projects archive", args: []string{"--dir", dir, "--actor", humanID, "projects", "archive", projectID}, expect: expectJSONEnvelope})
        run(t, invocation{name: "projects archive --unarchive", cmdPath: "projects archive", args: []string{"--dir", dir, "--actor", humanID, "projects", "archive", projectID, "--unarchive"}, expect: expectJSONEnvelope})

        // identity: list/whoami/use + agent ensure.
        run(t, invocation{name: "identity list", cmdPath: "identity list", args: []string{"--dir", dir, "--actor", humanID, "identity", "list"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "identity whoami", cmdPath: "identity whoami", args: []string{"--dir", dir, "--actor", humanID, "identity", "whoami"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "identity use", cmdPath: "identity use", args: []string{"--dir", dir, "--actor", humanID, "identity", "use", humanID}, expect: expectJSONEnvelope})
        agentViaCreateID := mustActorID(t, run(t, invocation{name: "identity create (agent via --user)", cmdPath: "identity create", args: []string{"--dir", dir, "--actor", humanID, "identity", "create", "--name", "Agent via create", "--kind", "agent", "--user", humanID}, expect: expectJSONEnvelope}).env)
        run(t, invocation{
                name:    "identity agent ensure (explicit flags, --use=false)",
                cmdPath: "identity agent ensure",
                args:    []string{"--dir", dir, "--actor", humanID, "identity", "agent", "ensure", "--session", "integration-session", "--name", "AgentName", "--user", humanID, "--use=false"},
                expect:  expectJSONEnvelope,
        })

        // agent start (covers flags on agent start).
        run(t, invocation{name: "agent start", cmdPath: "agent start", args: []string{"--dir", dir, "--actor", humanID, "agent", "start", itemA, "--session", "integration-session", "--name", "AgentName", "--user", humanID, "--take-assigned"}, expect: expectJSONEnvelope})

        // items claim (covers --take-assigned and error path).
        // Use an agent within the same human user to avoid assignment lock while still exercising the "already assigned" guard.
        itemD := mustID(t, run(t, invocation{
                name:    "items create (for claim test)",
                cmdPath: "items create",
                args:    []string{"--dir", dir, "--actor", humanID, "items", "create", "--project", projectID, "--outline", out1, "--title", "Item D"},
                expect:  expectJSONEnvelope,
        }).env)
        run(t, invocation{name: "items set-assign (assign to agent)", cmdPath: "items set-assign", args: []string{"--dir", dir, "--actor", humanID, "items", "set-assign", itemD, "--assignee", agentViaCreateID}, expect: expectJSONEnvelope})
        run(t, invocation{name: "items claim (expect error, already assigned)", cmdPath: "items claim", args: []string{"--dir", dir, "--actor", humanID, "items", "claim", itemD}, expect: expectError})
        run(t, invocation{name: "items claim --take-assigned", cmdPath: "items claim", args: []string{"--dir", dir, "--actor", humanID, "items", "claim", itemD, "--take-assigned"}, expect: expectJSONEnvelope})

        // docs: topics, topic (json), and raw markdown (non-envelope).
        run(t, invocation{name: "docs (topics)", cmdPath: "docs", args: []string{"docs"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "docs output-contract", cmdPath: "docs", args: []string{"docs", "output-contract"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "docs --raw", cmdPath: "docs", args: []string{"docs", "output-contract", "--raw"}, expect: expectRawText})
        run(t, invocation{name: "docs unknown topic", cmdPath: "docs", args: []string{"docs", "no-such-topic"}, expect: expectError})

        // Root persistent flags: --pretty and --format edn (verify envelopes).
        run(t, invocation{name: "pretty JSON", cmdPath: "status", args: []string{"--dir", dir, "--actor", humanID, "--pretty", "status"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "EDN output", cmdPath: "status", args: []string{"--dir", dir, "--actor", humanID, "--format", "edn", "status"}, expect: expectEDNEnvelope})

        // Also exercise env-based persistent option: CLARITY_DIR, CLARITY_ACTOR, CLARITY_WORKSPACE, CLARITY_FORMAT.
        // Use a fresh dir store for env-only tests.
        dir2 := t.TempDir()
        run(t, invocation{name: "init (env CLARITY_DIR)", cmdPath: "init", args: []string{"init"}, env: map[string]string{"CLARITY_DIR": dir2}, expect: expectJSONEnvelope, markEnvFlags: []string{"dir"}})
        actor2 := mustActorID(t, run(t, invocation{name: "identity create (env CLARITY_DIR)", cmdPath: "identity create", args: []string{"identity", "create", "--name", "Env Human", "--kind", "human", "--use"}, env: map[string]string{"CLARITY_DIR": dir2}, expect: expectJSONEnvelope, markEnvFlags: []string{"dir"}}).env)
        run(t, invocation{name: "status (env CLARITY_DIR + CLARITY_ACTOR)", cmdPath: "status", args: []string{"status"}, env: map[string]string{"CLARITY_DIR": dir2, "CLARITY_ACTOR": actor2}, expect: expectJSONEnvelope, markEnvFlags: []string{"dir", "actor"}})
        run(t, invocation{name: "status (env CLARITY_FORMAT=edn)", cmdPath: "status", args: []string{"status"}, env: map[string]string{"CLARITY_DIR": dir2, "CLARITY_ACTOR": actor2, "CLARITY_FORMAT": "edn"}, expect: expectEDNEnvelope, markEnvFlags: []string{"dir", "actor", "format"}})

        // Workspace-first path: exercise --workspace and the workspace command surface, using CLARITY_CONFIG_DIR temp override.
        // Initialize a workspace and set it current.
        wsName := "ws-integration"
        run(t, invocation{name: "workspace init", cmdPath: "workspace init", args: []string{"workspace", "init", wsName}, expect: expectJSONEnvelope})
        // Create identity inside workspace using --workspace (no --dir).
        wsHuman := mustActorID(t, run(t, invocation{
                name:    "identity create (workspace mode via --workspace)",
                cmdPath: "identity create",
                args:    []string{"--workspace", wsName, "identity", "create", "--name", "WS Human", "--kind", "human", "--use"},
                expect:  expectJSONEnvelope,
        }).env)

        // workspace current/list/use/rename
        run(t, invocation{name: "workspace current", cmdPath: "workspace current", args: []string{"workspace", "current"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "workspace list", cmdPath: "workspace list", args: []string{"workspace", "list"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "workspace use", cmdPath: "workspace use", args: []string{"workspace", "use", wsName}, expect: expectJSONEnvelope})
        wsName2 := "ws-integration-2"
        run(t, invocation{name: "workspace rename", cmdPath: "workspace rename", args: []string{"workspace", "rename", wsName, wsName2}, expect: expectJSONEnvelope})

        // workspace add/forget (global workspace registry; used for Git-backed workspaces).
        regName := "ws-registered"
        regDir := t.TempDir()
        run(t, invocation{name: "workspace add (--dir --kind --use)", cmdPath: "workspace add", args: []string{"workspace", "add", regName, "--dir", regDir, "--kind", "git", "--use"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "workspace forget", cmdPath: "workspace forget", args: []string{"workspace", "forget", regName}, expect: expectJSONEnvelope})

        // workspace export/import (+ flags).
        exportDir := t.TempDir()
        run(t, invocation{name: "workspace export (defaults, --to)", cmdPath: "workspace export", args: []string{"--workspace", wsName2, "--actor", wsHuman, "workspace", "export", "--to", exportDir}, expect: expectJSONEnvelope})
        // Force overwrite + exclude events.
        // Create a sentinel file to make directory non-empty.
        _ = writeFile(t, exportDir, "sentinel.txt", []byte("x"))
        // Negative: exporting to non-empty directory without --force should fail.
        run(t, invocation{name: "workspace export (non-empty without --force)", cmdPath: "workspace export", args: []string{"--workspace", wsName2, "--actor", wsHuman, "workspace", "export", "--to", exportDir, "--events=false"}, expect: expectError})
        run(t, invocation{name: "workspace export --force --events=false", cmdPath: "workspace export", args: []string{"--workspace", wsName2, "--actor", wsHuman, "workspace", "export", "--to", exportDir, "--force", "--events=false"}, expect: expectJSONEnvelope})
        // Import into a new workspace using positional name.
        importName := "ws-imported"
        run(t, invocation{name: "workspace import (positional) --from --use", cmdPath: "workspace import", args: []string{"workspace", "import", importName, "--from", exportDir, "--use"}, expect: expectJSONEnvelope})
        // Negative: import missing --from should fail.
        run(t, invocation{name: "workspace import (missing --from)", cmdPath: "workspace import", args: []string{"workspace", "import", "ws-missing-from"}, expect: expectError})
        // Import again using --name and --force to cover those flags.
        run(t, invocation{name: "workspace import --name --force --events=false", cmdPath: "workspace import", args: []string{"workspace", "import", "--name", importName, "--from", exportDir, "--force", "--events=false"}, expect: expectJSONEnvelope})

        // workspace migrate: migrate the sqlite-based temp store `dir` into a fresh workspace dir (and init+commit).
        migrateTo := t.TempDir()
        run(t, invocation{name: "workspace migrate (--from --to --git-init --git-commit --message)", cmdPath: "workspace migrate", args: []string{"workspace", "migrate", "--from", dir, "--to", migrateTo, "--git-init", "--git-commit", "--message", "clarity: migrate (test)"}, expect: expectJSONEnvelope})
        migrateTo2 := t.TempDir()
        run(t, invocation{name: "workspace migrate (--register --use --name)", cmdPath: "workspace migrate", args: []string{"workspace", "migrate", "--from", dir, "--to", migrateTo2, "--register", "--use", "--name", "ws-migrated"}, expect: expectJSONEnvelope})

        // Also cover CLARITY_WORKSPACE env (without --workspace).
        run(t, invocation{name: "status (env CLARITY_WORKSPACE)", cmdPath: "status", args: []string{"status"}, env: map[string]string{"CLARITY_WORKSPACE": wsName2, "CLARITY_ACTOR": wsHuman}, expect: expectJSONEnvelope, markEnvFlags: []string{"workspace", "actor"}})

        // Canonical JSONL + derived SQLite maintenance commands.
        // Use --dir to keep this self-contained and avoid touching ~/.clarity.
        run(t, invocation{name: "reindex (--dir)", cmdPath: "reindex", args: []string{"--dir", dir, "reindex"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "doctor --fail (--dir)", cmdPath: "doctor", args: []string{"--dir", dir, "doctor", "--fail"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "sync status (--dir)", cmdPath: "sync status", args: []string{"--dir", dir, "sync", "status"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "sync pull (non-repo error, --dir)", cmdPath: "sync pull", args: []string{"--dir", dir, "sync", "pull"}, expect: expectError})
        run(t, invocation{name: "sync push (non-repo error, --message, --pull=false, --dir)", cmdPath: "sync push", args: []string{"--dir", dir, "sync", "push", "--message", "test", "--pull=false"}, expect: expectError})
        run(t, invocation{name: "sync resolve (non-repo error, --dir)", cmdPath: "sync resolve", args: []string{"--dir", dir, "sync", "resolve"}, expect: expectError})
        run(t, invocation{name: "sync setup (--dir)", cmdPath: "sync setup", args: []string{"--dir", dir, "sync", "setup", "--commit=false", "--push=false"}, expect: expectJSONEnvelope})
        run(t, invocation{name: "sync setup (--remote-url/--remote-name/--message)", cmdPath: "sync setup", args: []string{"--dir", dir, "sync", "setup", "--remote-name", "origin", "--remote-url", "https://example.com/repo.git", "--message", "clarity: setup (test)", "--commit=false", "--push=false"}, expect: expectJSONEnvelope})

        // web: long-running server command; cover flags via --help (no server start).
        run(t, invocation{name: "web --help (--addr --read-only --auth --components-dir)", cmdPath: "web", args: []string{"--dir", dir, "web", "--addr", "127.0.0.1:0", "--read-only=false", "--auth", "magic", "--components-dir", dir, "--help"}, expect: expectRawText})

        // --- Coverage assertions ---
        leafCmds, rootPersistentFlags, localFlagsByCmd := buildCoverageIndex()
        assertCoverage(t, coveredCmds, coveredFlags, coveredLocalFlagsByCmd, leafCmds, rootPersistentFlags, localFlagsByCmd)
}

// buildCoverageIndex traverses the compiled Cobra command tree and returns:
// - all leaf command paths that should be exercised
// - root persistent flags (global options; covered across the whole suite)
// - local flags per leaf command (command-specific options; must be used with that command at least once)
func buildCoverageIndex() (leafCmds []string, rootPersistentFlags []string, localFlagsByCmd map[string][]string) {
        root := NewRootCmd()
        localFlagsByCmd = map[string][]string{}

        root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
                if f.Hidden || f.Name == "help" {
                        return
                }
                rootPersistentFlags = append(rootPersistentFlags, f.Name)
        })

        var walk func(c *cobra.Command)
        walk = func(c *cobra.Command) {
                if (c.Run != nil || c.RunE != nil) && !c.HasSubCommands() {
                        path := strings.TrimSpace(strings.TrimPrefix(c.CommandPath(), "clarity"))
                        path = strings.TrimSpace(path)
                        leafCmds = append(leafCmds, path)

                        var locals []string
                        c.Flags().VisitAll(func(f *pflag.Flag) {
                                if f.Hidden || f.Name == "help" {
                                        return
                                }
                                locals = append(locals, f.Name)
                        })
                        c.PersistentFlags().VisitAll(func(f *pflag.Flag) {
                                if f.Hidden || f.Name == "help" {
                                        return
                                }
                                locals = append(locals, f.Name)
                        })
                        localFlagsByCmd[path] = dedupeSorted(locals)
                }

                for _, sub := range c.Commands() {
                        if sub.Name() == "help" {
                                continue
                        }
                        walk(sub)
                }
        }
        walk(root)

        leafCmds = dedupeSorted(leafCmds)
        rootPersistentFlags = dedupeSorted(rootPersistentFlags)
        return leafCmds, rootPersistentFlags, localFlagsByCmd
}

func assertCoverage(
        t *testing.T,
        coveredCmds map[string]bool,
        coveredFlags map[string]bool,
        coveredLocalFlagsByCmd map[string]map[string]bool,
        leafCmds []string,
        rootPersistentFlags []string,
        localFlagsByCmd map[string][]string,
) {
        t.Helper()

        var missingCmds []string
        for _, leaf := range leafCmds {
                if leaf == "" {
                        // Root TUI: intentionally excluded from integration tests.
                        continue
                }
                if leaf == "capture" {
                        // Interactive TUI (requires terminal); excluded from non-interactive integration tests.
                        continue
                }
                if !coveredCmds[leaf] {
                        missingCmds = append(missingCmds, leaf)
                }
        }
        if len(missingCmds) > 0 {
                t.Fatalf("uncovered leaf commands (%d): %v", len(missingCmds), missingCmds)
        }

        var missingRootFlags []string
        for _, f := range rootPersistentFlags {
                if !coveredFlags[f] {
                        missingRootFlags = append(missingRootFlags, f)
                }
        }
        if len(missingRootFlags) > 0 {
                t.Fatalf("uncovered root persistent flags (%d): %v", len(missingRootFlags), missingRootFlags)
        }

        var missingLocal []string
        for _, leaf := range leafCmds {
                if leaf == "" {
                        continue
                }
                req := localFlagsByCmd[leaf]
                if len(req) == 0 {
                        continue
                }
                have := coveredLocalFlagsByCmd[leaf]
                for _, f := range req {
                        if have == nil || !have[f] {
                                missingLocal = append(missingLocal, fmt.Sprintf("%s: --%s", leaf, f))
                        }
                }
        }
        if len(missingLocal) > 0 {
                t.Fatalf("uncovered command-local flags (%d): %v", len(missingLocal), missingLocal)
        }
}

func assertJSONEnvelope(t *testing.T, stdout []byte, args []string) map[string]any {
        t.Helper()
        var env map[string]any
        if err := json.Unmarshal(stdout, &env); err != nil {
                t.Fatalf("stdout is not valid JSON\nargs: %v\nerr: %v\nstdout:\n%s", args, err, string(stdout))
        }
        // Allowed top-level keys.
        for k := range env {
                if k != "data" && k != "meta" && k != "_hints" {
                        t.Fatalf("unexpected top-level key %q in JSON envelope\nargs: %v\nenv: %#v", k, args, env)
                }
        }
        if _, ok := env["data"]; !ok {
                t.Fatalf("missing required top-level key \"data\" in JSON envelope\nargs: %v\nenv: %#v", args, env)
        }
        if v, ok := env["meta"]; ok && v != nil {
                if _, ok := v.(map[string]any); !ok {
                        t.Fatalf("meta must be an object when present\nargs: %v\nmeta: %#v", args, v)
                }
        }
        if v, ok := env["_hints"]; ok && v != nil {
                xs, ok := v.([]any)
                if !ok {
                        t.Fatalf("_hints must be an array of strings when present\nargs: %v\n_hints: %#v", args, v)
                }
                for i, it := range xs {
                        s, ok := it.(string)
                        if !ok || strings.TrimSpace(s) == "" {
                                t.Fatalf("_hints[%d] must be a non-empty string\nargs: %v\n_hints: %#v", i, args, v)
                        }
                }
        }
        return env
}

func assertEDNEnvelope(t *testing.T, stdout []byte, args []string) {
        t.Helper()
        s := strings.TrimSpace(string(stdout))
        if s == "" {
                t.Fatalf("expected EDN output\nargs: %v", args)
        }
        // Our EDN writer always writes a map for envelopes.
        if !strings.HasPrefix(s, "{") {
                t.Fatalf("expected EDN map\nargs: %v\nstdout:\n%s", args, string(stdout))
        }
        // Basic envelope keys should be present as keywords.
        if !strings.Contains(s, ":data") {
                t.Fatalf("expected EDN envelope to contain :data\nargs: %v\nstdout:\n%s", args, string(stdout))
        }
}

func assertDataIsSlice(t *testing.T, env map[string]any) []any {
        t.Helper()
        xs, ok := env["data"].([]any)
        if !ok {
                t.Fatalf("expected data to be an array; got: %#v", env["data"])
        }
        return xs
}

func assertDataMapHasKeys(t *testing.T, env map[string]any, keys ...string) map[string]any {
        t.Helper()
        m, ok := env["data"].(map[string]any)
        if !ok {
                t.Fatalf("expected data to be an object; got: %#v", env["data"])
        }
        for _, k := range keys {
                if _, ok := m[k]; !ok {
                        t.Fatalf("expected data.%s to exist; data: %#v", k, m)
                }
        }
        return m
}

func assertItemsShowEnvelope(t *testing.T, env map[string]any) {
        t.Helper()
        data := assertDataMapHasKeys(t, env, "item", "children", "deps")

        item, ok := data["item"].(map[string]any)
        if !ok {
                t.Fatalf("expected data.item object; got: %#v", data["item"])
        }
        for _, k := range []string{"id", "projectId", "outlineId", "title", "status", "ownerActorId", "createdBy", "createdAt", "updatedAt"} {
                // Note: item JSON tags use statusId in model, but showItem childSummary uses "status".
                // The item itself is model.Item => "status".
                if _, ok := item[k]; !ok {
                        // Also allow statusId for forward/back compat.
                        if k == "status" {
                                if _, ok := item["statusId"]; ok {
                                        continue
                                }
                        }
                        t.Fatalf("expected data.item.%s to exist; item: %#v", k, item)
                }
        }

        children, ok := data["children"].(map[string]any)
        if !ok {
                t.Fatalf("expected data.children object; got: %#v", data["children"])
        }
        if _, ok := children["items"].([]any); !ok {
                t.Fatalf("expected data.children.items array; got: %#v", children["items"])
        }
        counts, ok := children["counts"].(map[string]any)
        if !ok {
                t.Fatalf("expected data.children.counts object; got: %#v", children["counts"])
        }
        for _, k := range []string{"total", "open", "done", "archived"} {
                if _, ok := counts[k]; !ok {
                        t.Fatalf("expected data.children.counts.%s; counts: %#v", k, counts)
                }
        }

        deps, ok := data["deps"].(map[string]any)
        if !ok {
                t.Fatalf("expected data.deps object; got: %#v", data["deps"])
        }
        if _, ok := deps["blocks"].(map[string]any); !ok {
                t.Fatalf("expected data.deps.blocks object; got: %#v", deps["blocks"])
        }
        if _, ok := deps["related"].([]any); !ok {
                t.Fatalf("expected data.deps.related array; got: %#v", deps["related"])
        }

        // meta in items show should exist and have expected nested objects when present.
        if meta, ok := env["meta"].(map[string]any); ok {
                for _, k := range []string{"comments", "worklog", "deps"} {
                        if _, ok := meta[k]; !ok {
                                t.Fatalf("expected meta.%s to exist on items show; meta: %#v", k, meta)
                        }
                }
        }
}

func flagsFromArgs(args []string) []string {
        var out []string
        for _, a := range args {
                if !strings.HasPrefix(a, "--") {
                        continue
                }
                name := strings.TrimPrefix(a, "--")
                if eq := strings.IndexByte(name, '='); eq >= 0 {
                        name = name[:eq]
                }
                name = strings.TrimSpace(name)
                if name != "" {
                        out = append(out, name)
                }
        }
        return out
}

func mustID(t *testing.T, env map[string]any) string {
        t.Helper()
        d, ok := env["data"].(map[string]any)
        if !ok {
                t.Fatalf("expected data object; got: %#v", env["data"])
        }
        id, _ := d["id"].(string)
        if strings.TrimSpace(id) == "" {
                t.Fatalf("expected data.id; got: %#v", d)
        }
        return id
}

func mustActorID(t *testing.T, env map[string]any) string {
        return mustID(t, env)
}

func mustStatusLabels(t *testing.T, env map[string]any) []string {
        t.Helper()
        d, ok := env["data"].(map[string]any)
        if !ok {
                t.Fatalf("expected data object; got: %#v", env["data"])
        }
        defs, ok := d["statusDefs"].([]any)
        if !ok || len(defs) == 0 {
                t.Fatalf("expected data.statusDefs list; got: %#v", d["statusDefs"])
        }
        var labels []string
        for _, it := range defs {
                m, ok := it.(map[string]any)
                if !ok {
                        continue
                }
                l, _ := m["label"].(string)
                if strings.TrimSpace(l) != "" {
                        labels = append(labels, l)
                }
        }
        if len(labels) == 0 {
                t.Fatalf("expected at least one status label; got: %#v", defs)
        }
        return labels
}

func assertPaginatedListMeta(t *testing.T, env map[string]any) {
        t.Helper()
        meta, ok := env["meta"].(map[string]any)
        if !ok {
                t.Fatalf("expected meta object for paginated list; got: %#v", env["meta"])
        }
        // Presence checks. Types are float64 due to JSON decoding; we only validate existence.
        for _, k := range []string{"total", "limit", "offset", "returned"} {
                if _, ok := meta[k]; !ok {
                        t.Fatalf("missing meta.%s in paginated list envelope; meta: %#v", k, meta)
                }
        }
}

func dedupeSorted(xs []string) []string {
        sort.Strings(xs)
        out := make([]string, 0, len(xs))
        var prev string
        for i, x := range xs {
                if i == 0 || x != prev {
                        out = append(out, x)
                }
                prev = x
        }
        return out
}

func writeFile(t *testing.T, dir, name string, b []byte) error {
        t.Helper()
        path := dir + string(os.PathSeparator) + name
        return os.WriteFile(path, b, 0o644)
}
