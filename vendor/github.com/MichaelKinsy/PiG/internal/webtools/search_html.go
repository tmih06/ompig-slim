package webtools

// HTML DOM helpers for the search scrapers.
//
// golang.org/x/net/html is vendored, so the scrapers walk the tree instead of
// regexing markup. The names are prefixed `searchDOM` on purpose: the fetch
// pipeline has its own HTML helpers in this package and the two must not
// collide.

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// searchDOMTree parses a document body into a node tree.
func searchDOMTree(body []byte) (*html.Node, error) {
	return html.Parse(bytes.NewReader(body))
}

// searchDOMWalk visits root and its descendants depth-first. fn returning false
// skips that subtree.
func searchDOMWalk(root *html.Node, fn func(*html.Node) bool) {
	if root == nil {
		return
	}
	if !fn(root) {
		return
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		searchDOMWalk(child, fn)
	}
}

// searchDOMHasClass reports whether the node carries the exact class token.
func searchDOMHasClass(node *html.Node, class string) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}
	for _, field := range strings.Fields(searchDOMAttr(node, "class")) {
		if field == class {
			return true
		}
	}
	return false
}

// searchDOMAttr returns an attribute value, or "".
func searchDOMAttr(node *html.Node, name string) string {
	if node == nil {
		return ""
	}
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

// searchDOMFirstClass finds the first descendant element carrying class.
func searchDOMFirstClass(root *html.Node, class string) *html.Node {
	var found *html.Node
	searchDOMWalk(root, func(node *html.Node) bool {
		if found != nil {
			return false
		}
		if searchDOMHasClass(node, class) {
			found = node
			return false
		}
		return true
	})
	return found
}

// searchDOMText returns the collapsed text content of a subtree, skipping
// script and style bodies.
func searchDOMText(node *html.Node) string {
	if node == nil {
		return ""
	}
	var builder strings.Builder
	searchDOMWalk(node, func(n *html.Node) bool {
		switch n.Type {
		case html.TextNode:
			builder.WriteString(n.Data)
		case html.ElementNode:
			switch n.Data {
			case "script", "style":
				return false
			}
			builder.WriteByte(' ')
		}
		return true
	})
	return normalizeSearchText(builder.String())
}

// searchDOMClassElements lists the descendant elements that carry class, in
// document order.
func searchDOMClassElements(root *html.Node, class string) []*html.Node {
	nodes := make([]*html.Node, 0, 16)
	searchDOMWalk(root, func(node *html.Node) bool {
		if node != root && searchDOMHasClass(node, class) {
			nodes = append(nodes, node)
		}
		return true
	})
	return nodes
}
