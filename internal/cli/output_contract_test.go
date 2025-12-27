package cli

import (
        "encoding/json"
        "testing"
)

func TestOutputContract_JSONEnvelope_DefaultSuite(t *testing.T) {
        t.Setenv("CLARITY_CONFIG_DIR", t.TempDir())

        dir := t.TempDir()

        mustEnv := func(args ...string) map[string]any {
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
                if meta, ok := env["meta"]; ok && meta != nil {
                        if _, ok := meta.(map[string]any); !ok {
                                t.Fatalf("expected meta to be object; got %T", meta)
                        }
                }
                if hints, ok := env["_hints"]; ok && hints != nil {
                        if _, ok := hints.([]any); !ok {
                                t.Fatalf("expected _hints to be list; got %T", hints)
                        }
                }
                return env
        }

        // Dir-first commands.
        mustEnv("--dir", dir, "init")
        ident := mustEnv("--dir", dir, "identity", "create", "--name", "Output Contract Human", "--kind", "human", "--use")
        humanID, _ := ident["data"].(map[string]any)["id"].(string)
        if humanID == "" {
                t.Fatalf("expected identity create to return actor id; got: %#v", ident["data"])
        }
        proj := mustEnv("--dir", dir, "--actor", humanID, "projects", "create", "--name", "Output Contract Project", "--use")
        projectID, _ := proj["data"].(map[string]any)["id"].(string)
        if projectID == "" {
                t.Fatalf("expected projects create to return project id; got: %#v", proj["data"])
        }

        item := mustEnv("--dir", dir, "--actor", humanID, "items", "create", "--project", projectID, "--title", "Output Contract Item")
        itemID, _ := item["data"].(map[string]any)["id"].(string)
        if itemID == "" {
                t.Fatalf("expected items create to return item id; got: %#v", item["data"])
        }

        mustEnv("--dir", dir, "--actor", humanID, "status")
        mustEnv("--dir", dir, "--actor", humanID, "items", "show", itemID)
        mustEnv("--dir", dir, "--actor", humanID, "items", "list")
        mustEnv("--dir", dir, "--actor", humanID, "deps", "tree", itemID)

        // Workspace-first commands should respect CLARITY_CONFIG_DIR.
        wsName := "Output Contract Workspace"
        mustEnv("workspace", "init", wsName)
        wsHuman := mustEnv("--workspace", wsName, "identity", "create", "--name", "Workspace Human", "--kind", "human", "--use")
        wsHumanID, _ := wsHuman["data"].(map[string]any)["id"].(string)
        if wsHumanID == "" {
                t.Fatalf("expected identity create to return actor id; got: %#v", wsHuman["data"])
        }
        mustEnv("workspace", "current")
        mustEnv("workspace", "list")
        mustEnv("--workspace", wsName, "--actor", wsHumanID, "status")
}
