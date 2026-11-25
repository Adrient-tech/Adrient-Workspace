package wiki

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
)

type node = map[string]any

func para(text string) node {
	if text == "" {
		return node{"type": "paragraph"}
	}
	return node{"type": "paragraph", "content": []node{{"type": "text", "text": text}}}
}
func heading(l int, t string) node {
	return node{"type": "heading", "attrs": node{"level": l}, "content": []node{{"type": "text", "text": t}}}
}
func bullets(items ...string) node {
	var li []node
	for _, i := range items {
		li = append(li, node{"type": "listItem", "content": []node{para(i)}})
	}
	return node{"type": "bulletList", "content": li}
}
func tasks(items ...string) node {
	var li []node
	for _, i := range items {
		li = append(li, node{"type": "taskItem", "attrs": node{"checked": false}, "content": []node{para(i)}})
	}
	return node{"type": "taskList", "content": li}
}
func doc(n ...node) string {
	b, _ := json.Marshal(node{"type": "doc", "content": n})
	return string(b)
}

// TemplateNames lists the starter pages.
var TemplateNames = []struct{ ID, Name string }{
	{"", "Blank page"}, {"meeting_notes", "Meeting notes"}, {"project_brief", "Project brief"},
	{"weekly_plan", "Weekly plan"}, {"reading_list", "Reading list"}, {"task_board", "Task board (database)"},
}

// templateContent returns the document JSON, default title and icon of a template.
func templateContent(id string) (content, title, icon string, err error) {
	switch id {
	case "":
		return emptyDoc, "", "", nil
	case "welcome":
		return doc(heading(2, "Welcome"), para("This space is for your team's notes, plans and decisions."),
			bullets("Type / for blocks and @ to link another page", "Drag pages in the sidebar to reorder them", "Add a database page for tasks, contacts or anything with rows")), "Welcome", "👋", nil
	case "meeting_notes":
		return doc(heading(2, "Attendees"), bullets(""), heading(2, "Agenda"), bullets(""), heading(2, "Notes"), para(""), heading(2, "Action items"), tasks("")), "Meeting notes", "🗒️", nil
	case "project_brief":
		return doc(heading(2, "Goal"), para(""), heading(2, "Scope"), bullets("In scope", "Out of scope"), heading(2, "Milestones"), tasks("Kick-off", "First release"), heading(2, "Risks"), bullets("")), "Project brief", "🎯", nil
	case "weekly_plan":
		return doc(heading(2, "Priorities"), tasks("", "", ""), heading(2, "Monday"), para(""), heading(2, "Tuesday"), para(""), heading(2, "Wednesday"), para(""), heading(2, "Thursday"), para(""), heading(2, "Friday"), para("")), "Weekly plan", "📅", nil
	case "reading_list":
		return doc(heading(2, "To read"), tasks("", ""), heading(2, "Notes"), para("")), "Reading list", "📚", nil
	case "task_board":
		return emptyDoc, "Task board", "✅", nil
	}
	return "", "", "", knowkit.Invalid("unknown template %q", id)
}

// extractText returns the plain text of a TipTap document, one block per line.
func extractText(raw []byte) string {
	var n node
	if json.Unmarshal(raw, &n) != nil {
		return ""
	}
	var sb strings.Builder
	var walk func(n node)
	walk = func(n node) {
		if t, _ := n["text"].(string); t != "" {
			sb.WriteString(t)
		}
		kids, _ := n["content"].([]any)
		for _, k := range kids {
			if m, ok := k.(map[string]any); ok {
				walk(m)
			}
		}
		switch n["type"] {
		case "paragraph", "heading", "listItem", "taskItem", "codeBlock", "blockquote", "tableRow":
			sb.WriteString("\n")
		case "tableCell", "tableHeader":
			sb.WriteString("\t")
		}
	}
	walk(n)
	s := sb.String()
	if len(s) > 400000 {
		s = s[:400000]
	}
	return strings.TrimSpace(s)
}

var rePageHref = regexp.MustCompile(`^/wiki\?page=([0-9a-fA-F-]{36})`)

// extractPageLinks finds links to other wiki pages: link marks whose href is /wiki?page=<id>
// and "pageLink" nodes (attrs.id).
func extractPageLinks(raw []byte) []string {
	var n node
	if json.Unmarshal(raw, &n) != nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	var walk func(n node)
	walk = func(n node) {
		if n["type"] == "pageLink" {
			if at, ok := n["attrs"].(map[string]any); ok {
				if id, _ := at["id"].(string); id != "" {
					add(id)
				}
			}
		}
		if marks, ok := n["marks"].([]any); ok {
			for _, m := range marks {
				mm, _ := m.(map[string]any)
				if mm["type"] == "link" {
					at, _ := mm["attrs"].(map[string]any)
					if h, _ := at["href"].(string); h != "" {
						if g := rePageHref.FindStringSubmatch(h); g != nil {
							add(g[1])
						}
					}
				}
			}
		}
		kids, _ := n["content"].([]any)
		for _, k := range kids {
			if m, ok := k.(map[string]any); ok {
				walk(m)
			}
		}
	}
	walk(n)
	return out
}

// ---- Markdown ----------------------------------------------------------------

var (
	reBold   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reItalic = regexp.MustCompile(`(?:\*([^*]+)\*|_([^_]+)_)`)
	reCode   = regexp.MustCompile("`([^`]+)`")
	reStrike = regexp.MustCompile(`~~([^~]+)~~`)
	reLinkMd = regexp.MustCompile(`\[([^\]]+)\]\(([^)\s]+)\)`)
	reOL     = regexp.MustCompile(`^(\d+)[.)]\s+(.*)$`)
	reTask   = regexp.MustCompile(`^[-*+]\s+\[([ xX])\]\s+(.*)$`)
	reUL     = regexp.MustCompile(`^[-*+]\s+(.*)$`)
	reHead   = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*$`)
)

// inline parses bold, italic, code, strike and links into text nodes with marks.
func inline(s string) []node {
	type span struct {
		start, end int
		n          node
	}
	var spans []span
	taken := make([]bool, len(s))
	find := func(re *regexp.Regexp, mk func(g []string) node) {
		for _, m := range re.FindAllStringSubmatchIndex(s, -1) {
			ok := true
			for i := m[0]; i < m[1]; i++ {
				if taken[i] {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			g := make([]string, len(m)/2)
			for i := range g {
				if m[2*i] >= 0 {
					g[i] = s[m[2*i]:m[2*i+1]]
				}
			}
			for i := m[0]; i < m[1]; i++ {
				taken[i] = true
			}
			spans = append(spans, span{m[0], m[1], mk(g)})
		}
	}
	text := func(t string, marks ...node) node {
		n := node{"type": "text", "text": t}
		if len(marks) > 0 {
			n["marks"] = marks
		}
		return n
	}
	find(reCode, func(g []string) node { return text(g[1], node{"type": "code"}) })
	find(reLinkMd, func(g []string) node {
		href := g[2]
		if !safeHref(href) {
			return text(g[1])
		}
		return text(g[1], node{"type": "link", "attrs": node{"href": href}})
	})
	find(reBold, func(g []string) node { return text(g[1], node{"type": "bold"}) })
	find(reStrike, func(g []string) node { return text(g[1], node{"type": "strike"}) })
	find(reItalic, func(g []string) node {
		t := g[1]
		if t == "" {
			t = g[2]
		}
		return text(t, node{"type": "italic"})
	})
	// order spans, fill gaps with plain text
	for i := 1; i < len(spans); i++ {
		for j := i; j > 0 && spans[j].start < spans[j-1].start; j-- {
			spans[j], spans[j-1] = spans[j-1], spans[j]
		}
	}
	var out []node
	pos := 0
	for _, sp := range spans {
		if sp.start > pos {
			out = append(out, text(s[pos:sp.start]))
		}
		out = append(out, sp.n)
		pos = sp.end
	}
	if pos < len(s) {
		out = append(out, text(s[pos:]))
	}
	return out
}

func safeHref(h string) bool {
	l := strings.ToLower(strings.TrimSpace(h))
	return strings.HasPrefix(l, "http://") || strings.HasPrefix(l, "https://") || strings.HasPrefix(l, "mailto:") || strings.HasPrefix(l, "/")
}

func block(typ string, attrs node, inl []node) node {
	n := node{"type": typ}
	if attrs != nil {
		n["attrs"] = attrs
	}
	if len(inl) > 0 {
		n["content"] = inl
	}
	return n
}

// MarkdownToDoc converts Markdown to TipTap JSON. Raw HTML is not interpreted; it becomes text.
func MarkdownToDoc(md string) (title string, content string) {
	md = strings.ReplaceAll(strings.ReplaceAll(md, "\r\n", "\n"), "\r", "\n")
	if len(md) > 2<<20 {
		md = md[:2<<20]
	}
	lines := strings.Split(md, "\n")
	var out []node
	var list *node
	listKind := ""
	flush := func() {
		if list != nil {
			out = append(out, *list)
			list, listKind = nil, ""
		}
	}
	add := func(kind string, item node) {
		if listKind != kind {
			flush()
			l := node{"type": kind, "content": []node{}}
			list, listKind = &l, kind
		}
		(*list)["content"] = append((*list)["content"].([]node), item)
	}
	for i := 0; i < len(lines); i++ {
		ln := strings.TrimRight(lines[i], " \t")
		t := strings.TrimSpace(ln)
		switch {
		case strings.HasPrefix(t, "```"):
			flush()
			var code []string
			for i++; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```"); i++ {
				code = append(code, lines[i])
			}
			var inl []node
			if len(code) > 0 {
				inl = []node{{"type": "text", "text": strings.Join(code, "\n")}}
			}
			out = append(out, block("codeBlock", nil, inl))
		case t == "":
			flush()
		case t == "---" || t == "***" || t == "___":
			flush()
			out = append(out, node{"type": "horizontalRule"})
		case reHead.MatchString(t):
			flush()
			g := reHead.FindStringSubmatch(t)
			lvl := min(len(g[1]), 3)
			if title == "" && len(g[1]) == 1 {
				title = g[2]
				continue
			}
			out = append(out, block("heading", node{"level": lvl}, inline(g[2])))
		case strings.HasPrefix(t, ">"):
			flush()
			out = append(out, node{"type": "blockquote", "content": []node{block("paragraph", nil, inline(strings.TrimSpace(strings.TrimPrefix(t, ">"))))}})
		case reTask.MatchString(t):
			g := reTask.FindStringSubmatch(t)
			add("taskList", node{"type": "taskItem", "attrs": node{"checked": g[1] != " "}, "content": []node{block("paragraph", nil, inline(g[2]))}})
		case reUL.MatchString(t):
			add("bulletList", node{"type": "listItem", "content": []node{block("paragraph", nil, inline(reUL.FindStringSubmatch(t)[1]))}})
		case reOL.MatchString(t):
			add("orderedList", node{"type": "listItem", "content": []node{block("paragraph", nil, inline(reOL.FindStringSubmatch(t)[2]))}})
		default:
			flush()
			// join soft-wrapped lines of one paragraph
			para := t
			for i+1 < len(lines) && strings.TrimSpace(lines[i+1]) != "" && !reHead.MatchString(strings.TrimSpace(lines[i+1])) &&
				!reUL.MatchString(strings.TrimSpace(lines[i+1])) && !reOL.MatchString(strings.TrimSpace(lines[i+1])) &&
				!strings.HasPrefix(strings.TrimSpace(lines[i+1]), "```") && !strings.HasPrefix(strings.TrimSpace(lines[i+1]), ">") {
				i++
				para += " " + strings.TrimSpace(lines[i])
			}
			out = append(out, block("paragraph", nil, inline(para)))
		}
	}
	flush()
	if len(out) == 0 {
		out = []node{{"type": "paragraph"}}
	}
	b, _ := json.Marshal(node{"type": "doc", "content": out})
	return title, string(b)
}

// DocToMarkdown renders a page as Markdown.
func DocToMarkdown(title string, raw []byte) string {
	var d node
	_ = json.Unmarshal(raw, &d)
	var sb strings.Builder
	if title != "" {
		sb.WriteString("# " + title + "\n\n")
	}
	var inl func(n node) string
	inl = func(n node) string {
		var s strings.Builder
		kids, _ := n["content"].([]any)
		for _, k := range kids {
			m, _ := k.(map[string]any)
			t, _ := m["text"].(string)
			if m["type"] == "hardBreak" {
				s.WriteString("  \n")
				continue
			}
			marks, _ := m["marks"].([]any)
			for _, mk := range marks {
				mm, _ := mk.(map[string]any)
				switch mm["type"] {
				case "bold":
					t = "**" + t + "**"
				case "italic":
					t = "*" + t + "*"
				case "strike":
					t = "~~" + t + "~~"
				case "code":
					t = "`" + t + "`"
				case "link":
					at, _ := mm["attrs"].(map[string]any)
					h, _ := at["href"].(string)
					t = "[" + t + "](" + h + ")"
				}
			}
			s.WriteString(t)
		}
		return s.String()
	}
	var blocks func(n node, prefix string)
	blocks = func(n node, prefix string) {
		kids, _ := n["content"].([]any)
		for idx, k := range kids {
			m, _ := k.(map[string]any)
			switch m["type"] {
			case "heading":
				at, _ := m["attrs"].(map[string]any)
				l, _ := at["level"].(float64)
				sb.WriteString(prefix + strings.Repeat("#", max(int(l), 1)) + " " + inl(m) + "\n\n")
			case "paragraph":
				sb.WriteString(prefix + inl(m) + "\n\n")
			case "bulletList", "orderedList", "taskList":
				items, _ := m["content"].([]any)
				for i, it := range items {
					im, _ := it.(map[string]any)
					mark := "- "
					if m["type"] == "orderedList" {
						mark = itoa(i+1) + ". "
					}
					if m["type"] == "taskList" {
						at, _ := im["attrs"].(map[string]any)
						if c, _ := at["checked"].(bool); c {
							mark = "- [x] "
						} else {
							mark = "- [ ] "
						}
					}
					first := true
					for _, c := range im["content"].([]any) {
						cm, _ := c.(map[string]any)
						if first && cm["type"] == "paragraph" {
							sb.WriteString(prefix + mark + inl(cm) + "\n")
							first = false
							continue
						}
						blocks(node{"content": []any{cm}}, prefix+"  ")
					}
				}
				sb.WriteString("\n")
			case "blockquote":
				sb.WriteString(prefix + "> " + strings.ReplaceAll(strings.TrimSpace(extractText(mustJSON(m))), "\n", "\n"+prefix+"> ") + "\n\n")
			case "codeBlock":
				sb.WriteString(prefix + "```\n" + extractText(mustJSON(m)) + "\n" + prefix + "```\n\n")
			case "horizontalRule":
				sb.WriteString(prefix + "---\n\n")
			default:
				if t := extractText(mustJSON(m)); t != "" {
					sb.WriteString(prefix + t + "\n\n")
				}
			}
			_ = idx
		}
	}
	blocks(d, "")
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// DocToHTML renders a page as a small standalone HTML document (for print to PDF).
func DocToHTML(title string, raw []byte) string {
	md := DocToMarkdown("", raw)
	var sb strings.Builder
	sb.WriteString("<!doctype html><meta charset=utf-8><title>" + html.EscapeString(title) + "</title><body style=\"font-family:system-ui;max-width:760px;margin:2rem auto\"><h1>" + html.EscapeString(title) + "</h1>")
	for _, ln := range strings.Split(md, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		e := html.EscapeString(ln)
		switch {
		case strings.HasPrefix(ln, "### "):
			sb.WriteString("<h4>" + strings.TrimPrefix(e, "### ") + "</h4>")
		case strings.HasPrefix(ln, "## "):
			sb.WriteString("<h3>" + strings.TrimPrefix(e, "## ") + "</h3>")
		case strings.HasPrefix(ln, "# "):
			sb.WriteString("<h2>" + strings.TrimPrefix(e, "# ") + "</h2>")
		default:
			sb.WriteString("<p>" + e + "</p>")
		}
	}
	return sb.String()
}
