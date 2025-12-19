package docs

import (
        "embed"
        "io/fs"
        "path/filepath"
        "sort"
        "strings"
)

//go:embed content/*.md
var contentFS embed.FS

func Topics() []string {
        entries, err := fs.Glob(contentFS, "content/*.md")
        if err != nil {
                return []string{}
        }
        var topics []string
        for _, path := range entries {
                base := filepath.Base(path)
                topic := strings.TrimSuffix(base, filepath.Ext(base))
                if topic != "" {
                        topics = append(topics, topic)
                }
        }
        sort.Strings(topics)
        return topics
}

func Get(topic string) (string, bool) {
        topic = strings.TrimSpace(topic)
        if topic == "" {
                return "", false
        }
        topic = strings.ToLower(topic)
        path := filepath.Join("content", topic+".md")
        b, err := contentFS.ReadFile(path)
        if err != nil {
                return "", false
        }
        return string(b), true
}
