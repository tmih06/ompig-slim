package webtools

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// RenderHTML converts an HTML document to Markdown with the native reader. It is
// the reader the fetch pipeline uses; the URL resolves relative links.
func RenderHTML(body []byte, baseURL string) string {
	return renderHTMLDocument(parseHTML(body), baseURL)
}

// parseHTML parses an HTML body. A body that does not parse still yields a
// document, since html.Parse only fails on read errors.
func parseHTML(body []byte) *html.Node {
	document, err := html.Parse(strings.NewReader(string(decodeBody(body, "text/html", "text/html"))))
	if err != nil || document == nil {
		return &html.Node{Type: html.DocumentNode}
	}
	return document
}

// HTMLTitle returns the document's title, preferring <title> over the first <h1>.
func HTMLTitle(document *html.Node) string {
	if title := textOfFirst(document, html.ElementNode, "title"); title != "" {
		return collapseSpace(title)
	}
	if heading := textOfFirst(document, html.ElementNode, "h1"); heading != "" {
		return collapseSpace(heading)
	}
	return ""
}

// textOfFirst returns the text under the first element with the given tag.
func textOfFirst(node *html.Node, kind html.NodeType, tag string) string {
	if found := findElement(node, tag); found != nil {
		return collectText(found)
	}
	return ""
}

// findElement returns the first element with the given tag in document order.
func findElement(node *html.Node, tag string) *html.Node {
	if node.Type == html.ElementNode && node.Data == tag {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if found := findElement(child, tag); found != nil {
			return found
		}
	}
	return nil
}

// collectText concatenates the text under a node, skipping script and style.
func collectText(node *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		switch n.Type {
		case html.TextNode:
			builder.WriteString(n.Data)
		case html.ElementNode:
			switch n.Data {
			case "script", "style", "noscript", "template":
				return
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return builder.String()
}

// markdownAlternate returns the href of a `<link rel="alternate">` that points at
// a Markdown rendering of the page.
func markdownAlternate(document *html.Node, baseURL string) (string, bool) {
	var found string
	walkElements(document, func(node *html.Node) bool {
		if node.Data != "link" {
			return found == ""
		}
		rel := attribute(node, "rel")
		if !hasWord(rel, "alternate") {
			return true
		}
		kind := baseMediaType(attribute(node, "type"))
		if kind != "text/markdown" && kind != "text/x-markdown" && !strings.HasSuffix(lowerExtension(attribute(node, "href")), ".md") {
			return true
		}
		href := resolveLink(attribute(node, "href"), baseURL)
		if href == "" || !isHTTPURL(href) {
			return true
		}
		found = href
		return false
	})
	return found, found != ""
}

// documentLink returns the href of the first linked PDF or DOCX the page offers,
// which is how a paper's landing page points at its text.
func documentLink(document *html.Node, baseURL string) (string, bool) {
	var found string
	walkElements(document, func(node *html.Node) bool {
		if node.Data != "a" {
			return found == ""
		}
		href := attribute(node, "href")
		switch lowerExtension(href) {
		case ".pdf", ".docx":
		default:
			return true
		}
		resolved := resolveLink(href, baseURL)
		if resolved == "" || !isHTTPURL(resolved) {
			return true
		}
		found = resolved
		return false
	})
	return found, found != ""
}

// walkElements visits element nodes in document order until visit returns false.
func walkElements(node *html.Node, visit func(*html.Node) bool) {
	if !visit(node) {
		return
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walkElements(child, visit)
	}
}

// attribute returns an element's attribute value, or "".
func attribute(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

// hasWord reports whether space-separated list contains word.
func hasWord(list, word string) bool {
	for _, field := range strings.Fields(list) {
		if strings.EqualFold(field, word) {
			return true
		}
	}
	return false
}

// qualityOK is the reader's behavioural gate: text made mostly of short lines is
// a menu or a navigation shell, not an article. It also enforces the minimum
// rendered length.
func qualityOK(markdown string) bool {
	trimmed := strings.TrimSpace(markdown)
	if len([]rune(trimmed)) < minRenderedChars {
		return false
	}
	lines := strings.Split(trimmed, "\n")
	long, short := 0, 0
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) < 4 {
			short++
		} else {
			long++
		}
	}
	if long == 0 {
		return false
	}
	// A page with more short lines than long ones is a link farm.
	return long*2 >= short || long >= 5
}

// renderHTMLDocument converts an HTML document to Markdown.
func renderHTMLDocument(document *html.Node, baseURL string) string {
	writer := &markdownWriter{baseURL: baseURL}
	writer.walk(document)
	return writer.finish()
}

// markdownWriter renders HTML to Markdown, keeping the structure the model needs
// (headings, paragraphs, lists, tables, code, links) and dropping the chrome
// (navigation, banners, scripts, forms, hidden elements).
type markdownWriter struct {
	baseURL string
	builder strings.Builder
	// pending is the run of whitespace the next inline write normalizes.
	pending    bool
	lineStart  bool
	inPre      int
	listDepth  int
	quoteDepth int
	// skip counts the dropped container depth; anything inside it is ignored.
	skip int
	// preDepth counts the open <pre> elements; whitespace is literal inside.
	tableDepth int
	cellCount  int
}

// droppedTags are elements whose whole subtree is chrome.
var droppedTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "template": true, "svg": true,
	"canvas": true, "iframe": true, "nav": true, "form": true, "button": true,
	"select": true, "option": true, "optgroup": true, "textarea": true, "input": true,
	"footer": true, "aside": true, "dialog": true, "menu": true,
}

// hiddenAttributes mark visual chrome that carries no document text.
func isHidden(node *html.Node) bool {
	if attribute(node, "hidden") != "" {
		return true
	}
	if strings.EqualFold(attribute(node, "aria-hidden"), "true") {
		return true
	}
	for _, key := range []string{"display:none", "visibility:hidden"} {
		if strings.Contains(strings.ReplaceAll(attribute(node, "style"), " ", ""), key) {
			return true
		}
	}
	return false
}

func (w *markdownWriter) walk(node *html.Node) {
	if w.skip > 0 {
		return
	}
	switch node.Type {
	case html.TextNode:
		w.text(node.Data)
		return
	case html.DocumentNode:
	case html.ElementNode:
		if w.enter(node) {
			return
		}
		defer w.leave(node)
	case html.CommentNode, html.DoctypeNode, html.ErrorNode:
		return
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		w.walk(child)
	}
}

// enter handles an element's opening. It reports true when the element was
// dropped, in which case its subtree is not walked.
func (w *markdownWriter) enter(node *html.Node) bool {
	tag := node.Data
	if droppedTags[tag] || isHidden(node) {
		return true
	}
	switch tag {
	case "head":
		return true
	case "h1", "h2", "h3", "h4", "h5", "h6":
		w.block()
		w.builder.WriteString(strings.Repeat("#", int(tag[1]-'0')))
		w.builder.WriteByte(' ')
		w.lineStart = false
	case "p", "div", "section", "article", "main", "header", "address", "figure",
		"figcaption", "dl", "dd", "dt", "fieldset", "legend":
		w.block()
	case "blockquote":
		w.block()
		w.quoteDepth++
		w.lineStart = true
	case "pre":
		w.block()
		w.builder.WriteString("```\n")
		w.inPre++
		w.lineStart = false
	case "code", "kbd", "samp", "tt":
		if w.inPre == 0 {
			w.builder.WriteByte('`')
		}
	case "strong", "b":
		w.builder.WriteString("**")
	case "em", "i", "cite", "var":
		w.builder.WriteByte('*')
	case "del", "s", "strike":
		w.builder.WriteString("~~")
	case "br":
		if w.inPre > 0 {
			w.builder.WriteByte('\n')
		} else {
			w.builder.WriteString("  \n")
		}
		w.lineStart = true
	case "hr":
		w.block()
		w.builder.WriteString("---\n")
		w.lineStart = true
	case "ul", "ol":
		w.block()
		w.listDepth++
	case "li":
		w.block()
		w.builder.WriteString(strings.Repeat("  ", max(w.listDepth-1, 0)))
		w.builder.WriteString("- ")
		w.lineStart = false
	case "a":
		w.pending = false
		href := resolveLink(attribute(node, "href"), w.baseURL)
		if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(strings.ToLower(href), "javascript:") {
			return false
		}
		w.builder.WriteByte('[')
	case "img":
		alt := strings.TrimSpace(attribute(node, "alt"))
		if alt == "" {
			return true
		}
		w.pending = false
		w.builder.WriteString("![" + alt + "](")
		w.builder.WriteString(resolveLink(attribute(node, "src"), w.baseURL))
		w.builder.WriteString(")")
		w.pending = true
		return true
	case "table":
		w.block()
		w.tableDepth++
		w.cellCount = 0
	case "tr":
		w.block()
		w.cellCount = 0
	case "td", "th":
		if w.cellCount > 0 {
			w.builder.WriteString(" | ")
		} else {
			w.builder.WriteString("| ")
		}
		w.cellCount++
		w.lineStart = false
	case "wbr":
		return true
	}
	return false
}

// leave handles an element's closing.
func (w *markdownWriter) leave(node *html.Node) {
	tag := node.Data
	switch tag {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		w.builder.WriteString("\n\n")
		w.lineStart = true
	case "pre":
		w.inPre--
		if !strings.HasSuffix(w.builder.String(), "\n") {
			w.builder.WriteByte('\n')
		}
		w.builder.WriteString("```\n\n")
		w.lineStart = true
	case "code", "kbd", "samp", "tt":
		if w.inPre == 0 {
			w.builder.WriteByte('`')
		}
	case "strong", "b":
		w.builder.WriteString("**")
	case "em", "i", "cite", "var":
		w.builder.WriteByte('*')
	case "del", "s", "strike":
		w.builder.WriteString("~~")
	case "blockquote":
		w.quoteDepth--
		w.block()
	case "ul", "ol":
		w.listDepth--
		w.block()
	case "a":
		href := resolveLink(attribute(node, "href"), w.baseURL)
		if href != "" && !strings.HasPrefix(href, "#") && !strings.HasPrefix(strings.ToLower(href), "javascript:") {
			w.builder.WriteString("](")
			w.builder.WriteString(href)
			w.builder.WriteByte(')')
			w.pending = true
		}
	case "td", "th":
	case "tr":
		if w.cellCount > 0 {
			w.builder.WriteString(" |\n")
			w.lineStart = true
		}
		w.cellCount = 0
	case "table":
		w.tableDepth--
		w.block()
	}
}

// block starts a new block: it trims trailing spaces and keeps exactly one blank
// line between blocks.
func (w *markdownWriter) block() {
	text := w.builder.String()
	trimmed := strings.TrimRight(text, " \t\n")
	if trimmed == "" {
		w.builder.Reset()
		w.lineStart = true
		w.pending = false
		return
	}
	if len(trimmed) != len(text) {
		w.builder.Reset()
		w.builder.WriteString(trimmed)
	}
	if !strings.HasSuffix(w.builder.String(), "\n\n") {
		if strings.HasSuffix(w.builder.String(), "\n") {
			w.builder.WriteByte('\n')
		} else {
			w.builder.WriteString("\n\n")
		}
	}
	w.lineStart = true
	w.pending = false
}

// text writes inline text, collapsing whitespace outside <pre>.
func (w *markdownWriter) text(value string) {
	if w.inPre > 0 {
		w.builder.WriteString(value)
		return
	}
	collapsed := collapseSpace(value)
	if collapsed == "" {
		if value != "" {
			w.pending = true
		}
		return
	}
	if w.pending && !w.lineStart && w.builder.Len() > 0 {
		w.builder.WriteByte(' ')
	}
	if w.lineStart {
		if w.quoteDepth > 0 {
			w.builder.WriteString(strings.Repeat("> ", w.quoteDepth))
		}
		w.lineStart = false
	}
	w.builder.WriteString(collapsed)
	if strings.HasSuffix(value, " ") {
		w.pending = true
	} else {
		w.pending = false
	}
}

// finish trims the document and guarantees one trailing newline.
func (w *markdownWriter) finish() string {
	text := strings.TrimSpace(w.builder.String())
	if text == "" {
		return ""
	}
	return text + "\n"
}

// collapseSpace folds every run of whitespace into one space and trims the ends.
func collapseSpace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// resolveLink resolves a document-relative href against the page URL. An empty
// href, or one whose scheme is not http(s), resolves to "".
func resolveLink(href, baseURL string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	lower := strings.ToLower(href)
	for _, prefix := range []string{"javascript:", "data:", "mailto:", "tel:"} {
		if strings.HasPrefix(lower, prefix) {
			return ""
		}
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return href
	}
	reference, err := url.Parse(href)
	if err != nil {
		return href
	}
	resolved := base.ResolveReference(reference)
	if resolved.Scheme != "http" && resolved.Scheme != "https" && resolved.Scheme != "file" {
		return ""
	}
	return resolved.String()
}

// isHTTPURL reports an absolute http(s) URL.
func isHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}
