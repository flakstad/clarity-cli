package web

import (
        "bytes"
        "html/template"
        "strings"

        "github.com/yuin/goldmark"
        emoji "github.com/yuin/goldmark-emoji"
        "github.com/yuin/goldmark/extension"
        "github.com/yuin/goldmark/renderer/html"
)

var markdownRenderer = goldmark.New(
        goldmark.WithExtensions(
                extension.GFM,
                emoji.Emoji,
        ),
        goldmark.WithRendererOptions(
                // Do not allow raw HTML passthrough (avoid XSS).
                // Note: we intentionally do NOT use html.WithUnsafe().
                html.WithHardWraps(),
        ),
)

func renderMarkdownHTML(src string) template.HTML {
        src = strings.TrimSpace(src)
        if src == "" {
                return template.HTML("")
        }
        var b bytes.Buffer
        if err := markdownRenderer.Convert([]byte(src), &b); err != nil {
                return template.HTML("<pre>" + template.HTMLEscapeString(src) + "</pre>")
        }
        // goldmark output is trusted only because raw HTML is disabled above.
        return template.HTML(b.String())
}
