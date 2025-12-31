package cli

import (
        "errors"
        "strings"
        "time"

        "clarity-cli/internal/model"
        "clarity-cli/internal/store"

        "github.com/spf13/cobra"
)

func newDepsCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "deps",
                Short: "Dependency commands",
        }
        cmd.AddCommand(newDepsAddCmd(app))
        cmd.AddCommand(newDepsListCmd(app))
        cmd.AddCommand(newDepsTreeCmd(app))
        cmd.AddCommand(newDepsCyclesCmd(app))
        return cmd
}

func newDepsAddCmd(app *App) *cobra.Command {
        var blocks string
        var related string

        cmd := &cobra.Command{
                Use:   "add <item-id>",
                Short: "Add a dependency (owner-only on the dependent item)",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, s, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        actorID, err := currentActorID(app, db)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        itemID := args[0]
                        t, ok := db.FindItem(itemID)
                        if !ok {
                                return writeErr(cmd, errNotFound("item", itemID))
                        }
                        if !canEditTask(db, actorID, t) {
                                return writeErr(cmd, errorsOwnerOnly(actorID, t.OwnerActorID, itemID))
                        }

                        var depType model.DependencyType
                        var targetID string
                        switch {
                        case blocks != "" && related != "":
                                return writeErr(cmd, errors.New("provide exactly one of --blocks or --related"))
                        case blocks != "":
                                depType = model.DependencyBlocks
                                targetID = blocks
                        case related != "":
                                depType = model.DependencyRelated
                                targetID = related
                        default:
                                return writeErr(cmd, errors.New("missing --blocks or --related"))
                        }

                        if _, ok := db.FindItem(targetID); !ok {
                                return writeErr(cmd, errNotFound("item", targetID))
                        }

                        d := model.Dependency{
                                ID:         s.NextID(db, "dep"),
                                FromItemID: itemID,
                                ToItemID:   targetID,
                                Type:       depType,
                                CreatedBy:  actorID,
                                CreatedAt:  time.Now().UTC(),
                        }
                        db.Deps = append(db.Deps, d)
                        if err := s.AppendEvent(actorID, "dep.add", d.ID, d); err != nil {
                                return writeErr(cmd, err)
                        }
                        if err := s.Save(db); err != nil {
                                return writeErr(cmd, err)
                        }
                        return writeOut(cmd, app, map[string]any{"data": d})
                },
        }

        cmd.Flags().StringVar(&blocks, "blocks", "", "Item id that blocks this item")
        cmd.Flags().StringVar(&related, "related", "", "Item id related to this item")
        return cmd
}

func newDepsListCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "list [item-id]",
                Short: "List dependencies (optionally for a single item)",
                Args:  cobra.RangeArgs(0, 1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }

                        if len(args) == 0 {
                                out := db.Deps
                                if out == nil {
                                        out = []model.Dependency{}
                                }
                                return writeOut(cmd, app, map[string]any{"data": out})
                        }
                        itemID := args[0]
                        out := make([]model.Dependency, 0)
                        for _, d := range db.Deps {
                                if d.FromItemID == itemID || d.ToItemID == itemID {
                                        out = append(out, d)
                                }
                        }
                        return writeOut(cmd, app, map[string]any{"data": out})
                },
        }
        return cmd
}

type depsTreeNode struct {
        ID       string         `json:"id"`
        Title    string         `json:"title"`
        StatusID string         `json:"status"`
        BlocksOn []depsTreeNode `json:"blocksOn,omitempty"`
}

func newDepsTreeCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "tree <item-id>",
                Short: "Show dependency tree (blocks) for an item",
                Args:  cobra.ExactArgs(1),
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        rootID := args[0]
                        rootTask, ok := db.FindItem(rootID)
                        if !ok {
                                return writeErr(cmd, errNotFound("item", rootID))
                        }

                        blocksGraph := buildBlocksGraph(db)
                        seen := map[string]bool{}
                        var build func(id string) depsTreeNode
                        build = func(id string) depsTreeNode {
                                t, _ := db.FindItem(id)
                                node := depsTreeNode{ID: id}
                                if t != nil {
                                        node.Title = t.Title
                                        node.StatusID = t.StatusID
                                }
                                if seen[id] {
                                        return node
                                }
                                seen[id] = true
                                for _, dep := range blocksGraph[id] {
                                        node.BlocksOn = append(node.BlocksOn, build(dep))
                                }
                                return node
                        }

                        out := depsTreeNode{
                                ID:       rootTask.ID,
                                Title:    rootTask.Title,
                                StatusID: rootTask.StatusID,
                        }
                        for _, dep := range blocksGraph[rootTask.ID] {
                                out.BlocksOn = append(out.BlocksOn, build(dep))
                        }

                        return writeOut(cmd, app, map[string]any{"data": out})
                },
        }
        return cmd
}

func newDepsCyclesCmd(app *App) *cobra.Command {
        cmd := &cobra.Command{
                Use:   "cycles",
                Short: "Detect cycles in the blocks dependency graph",
                RunE: func(cmd *cobra.Command, args []string) error {
                        db, _, err := loadDB(app)
                        if err != nil {
                                return writeErr(cmd, err)
                        }
                        graph := buildBlocksGraph(db)
                        cycles := findCycles(graph)
                        if cycles == nil {
                                cycles = [][]string{}
                        }
                        return writeOut(cmd, app, map[string]any{"data": cycles})
                },
        }
        return cmd
}

func buildBlocksGraph(db *store.DB) map[string][]string {
        graph := map[string][]string{}
        for _, d := range db.Deps {
                if d.Type != model.DependencyBlocks {
                        continue
                }
                graph[d.FromItemID] = append(graph[d.FromItemID], d.ToItemID)
        }
        return graph
}

func findCycles(graph map[string][]string) [][]string {
        visited := map[string]bool{}
        onStack := map[string]bool{}
        var stack []string
        var cycles [][]string
        seenCycleKey := map[string]bool{}

        var dfs func(n string)
        dfs = func(n string) {
                visited[n] = true
                onStack[n] = true
                stack = append(stack, n)

                for _, m := range graph[n] {
                        if !visited[m] {
                                dfs(m)
                                continue
                        }
                        if onStack[m] {
                                // Extract cycle from stack starting at m.
                                var cycle []string
                                for i := len(stack) - 1; i >= 0; i-- {
                                        cycle = append([]string{stack[i]}, cycle...)
                                        if stack[i] == m {
                                                break
                                        }
                                }
                                cycle = append(cycle, m)
                                key := strings.Join(cycle, "->")
                                if !seenCycleKey[key] {
                                        seenCycleKey[key] = true
                                        cycles = append(cycles, cycle)
                                }
                        }
                }

                stack = stack[:len(stack)-1]
                onStack[n] = false
        }

        for n := range graph {
                if !visited[n] {
                        dfs(n)
                }
        }
        return cycles
}
