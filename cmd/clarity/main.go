package main

import (
        "os"
        "strings"

        "clarity-cli/internal/cli"
)

func main() {
        // Convenience: `clarity <item-id>` works like `clarity items show <item-id>`.
        //
        // Cobra treats the first token as a subcommand, so we rewrite argv before parsing.
        if len(os.Args) >= 2 && strings.HasPrefix(strings.TrimSpace(os.Args[1]), "item-") {
                id := strings.TrimSpace(os.Args[1])
                os.Args = append([]string{os.Args[0], "items", "show", id}, os.Args[2:]...)
        }

        cmd := cli.NewRootCmd()
        if err := cmd.Execute(); err != nil {
                os.Exit(1)
        }
}
