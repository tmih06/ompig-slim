package codingagent

// The grep and find cards are drawn by the TUI card itself
// (tui.ToolExecutionComponent.SetSearchOutput): the grouped search tree lives
// with the other tree rendering in tui/tree_list.go, because the interactive
// card's shape must not depend on which renderer path resolved for a tool.
