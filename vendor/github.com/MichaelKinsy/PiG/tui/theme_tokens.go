package tui

// Ports packages/coding-agent/src/modes/interactive/theme/theme.ts (the ThemeColor and ThemeBg tokens).

// ThemeColor is a foreground theme token: upstream `ThemeColor` (modes/interactive/theme/theme.ts:57).
type ThemeColor string

// The ThemeColor tokens, in upstream order.
const (
	ThemeColorAccent             ThemeColor = "accent"
	ThemeColorBorder             ThemeColor = "border"
	ThemeColorBorderAccent       ThemeColor = "borderAccent"
	ThemeColorBorderMuted        ThemeColor = "borderMuted"
	ThemeColorSuccess            ThemeColor = "success"
	ThemeColorError              ThemeColor = "error"
	ThemeColorWarning            ThemeColor = "warning"
	ThemeColorMuted              ThemeColor = "muted"
	ThemeColorDim                ThemeColor = "dim"
	ThemeColorText               ThemeColor = "text"
	ThemeColorThinkingText       ThemeColor = "thinkingText"
	ThemeColorScrollbarTrack     ThemeColor = "scrollbarTrack"
	ThemeColorScrollbarThumb     ThemeColor = "scrollbarThumb"
	ThemeColorSearchMatchText    ThemeColor = "searchMatchText"
	ThemeColorUserMessageText    ThemeColor = "userMessageText"
	ThemeColorCustomMessageText  ThemeColor = "customMessageText"
	ThemeColorCustomMessageLabel ThemeColor = "customMessageLabel"
	ThemeColorToolTitle          ThemeColor = "toolTitle"
	ThemeColorToolOutput         ThemeColor = "toolOutput"
	ThemeColorMdHeading          ThemeColor = "mdHeading"
	ThemeColorMdLink             ThemeColor = "mdLink"
	ThemeColorMdLinkUrl          ThemeColor = "mdLinkUrl"
	ThemeColorMdCode             ThemeColor = "mdCode"
	ThemeColorMdCodeBlock        ThemeColor = "mdCodeBlock"
	ThemeColorMdCodeBlockBorder  ThemeColor = "mdCodeBlockBorder"
	ThemeColorMdQuote            ThemeColor = "mdQuote"
	ThemeColorMdQuoteBorder      ThemeColor = "mdQuoteBorder"
	ThemeColorMdHr               ThemeColor = "mdHr"
	ThemeColorMdListBullet       ThemeColor = "mdListBullet"
	ThemeColorToolDiffAdded      ThemeColor = "toolDiffAdded"
	ThemeColorToolDiffRemoved    ThemeColor = "toolDiffRemoved"
	ThemeColorToolDiffContext    ThemeColor = "toolDiffContext"
	ThemeColorSyntaxComment      ThemeColor = "syntaxComment"
	ThemeColorSyntaxKeyword      ThemeColor = "syntaxKeyword"
	ThemeColorSyntaxFunction     ThemeColor = "syntaxFunction"
	ThemeColorSyntaxVariable     ThemeColor = "syntaxVariable"
	ThemeColorSyntaxString       ThemeColor = "syntaxString"
	ThemeColorSyntaxNumber       ThemeColor = "syntaxNumber"
	ThemeColorSyntaxType         ThemeColor = "syntaxType"
	ThemeColorSyntaxOperator     ThemeColor = "syntaxOperator"
	ThemeColorSyntaxPunctuation  ThemeColor = "syntaxPunctuation"
	ThemeColorThinkingOff        ThemeColor = "thinkingOff"
	ThemeColorThinkingMinimal    ThemeColor = "thinkingMinimal"
	ThemeColorThinkingLow        ThemeColor = "thinkingLow"
	ThemeColorThinkingMedium     ThemeColor = "thinkingMedium"
	ThemeColorThinkingHigh       ThemeColor = "thinkingHigh"
	ThemeColorThinkingXhigh      ThemeColor = "thinkingXhigh"
	ThemeColorThinkingMax        ThemeColor = "thinkingMax"
	ThemeColorBashMode           ThemeColor = "bashMode"
	ThemeColorPythonMode         ThemeColor = "pythonMode"
	// ThemeColorAssistantMessageText colors the assistant reply's plain prose.
	// Unset means the terminal default, which is what every theme did before the
	// token existed; a theme sets it when it wants prose to carry its own body
	// color instead of inheriting whatever the user's terminal shows.
	ThemeColorAssistantMessageText ThemeColor = "assistantMessageText"

	// Status line segments. Upstream defines them in the same color object as the
	// tokens above; they are optional here, because a theme written for PiG omits
	// them and still has to render a status line (see themeColorFallbacks).
	ThemeColorStatusLineSep       ThemeColor = "statusLineSep"
	ThemeColorStatusLineModel     ThemeColor = "statusLineModel"
	ThemeColorStatusLinePath      ThemeColor = "statusLinePath"
	ThemeColorStatusLineGitClean  ThemeColor = "statusLineGitClean"
	ThemeColorStatusLineGitDirty  ThemeColor = "statusLineGitDirty"
	ThemeColorStatusLineContext   ThemeColor = "statusLineContext"
	ThemeColorStatusLineSpend     ThemeColor = "statusLineSpend"
	ThemeColorStatusLineStaged    ThemeColor = "statusLineStaged"
	ThemeColorStatusLineDirty     ThemeColor = "statusLineDirty"
	ThemeColorStatusLineUntracked ThemeColor = "statusLineUntracked"
	ThemeColorStatusLineOutput    ThemeColor = "statusLineOutput"
	ThemeColorStatusLineCost      ThemeColor = "statusLineCost"
	ThemeColorStatusLineSubagents ThemeColor = "statusLineSubagents"
)

// ThemeBg is a background theme token: upstream `ThemeBg` (theme.ts:108).
type ThemeBg string

// The ThemeBg tokens, in upstream order.
const (
	ThemeBgSelectedBg      ThemeBg = "selectedBg"
	ThemeBgSearchMatchBg   ThemeBg = "searchMatchBg"
	ThemeBgUserMessageBg   ThemeBg = "userMessageBg"
	ThemeBgCustomMessageBg ThemeBg = "customMessageBg"
	ThemeBgToolPendingBg   ThemeBg = "toolPendingBg"
	ThemeBgToolSuccessBg   ThemeBg = "toolSuccessBg"
	ThemeBgToolErrorBg     ThemeBg = "toolErrorBg"
)

// ANSI palette slots a theme's "terminal.ansi" array names, in index order: the
// hues colorful Markdown paints headings, emphasis, inline code and table
// headers with (upstream packages/tui/src/theme/schema.ts ANSI_COLOR_INDEX).
const (
	ANSIBlack = iota
	ANSIRed
	ANSIGreen
	ANSIYellow
	ANSIBlue
	ANSIMagenta
	ANSICyan
	ANSIWhite
	ANSIBrightBlack
	ANSIBrightRed
	ANSIBrightGreen
	ANSIBrightYellow
	ANSIBrightBlue
	ANSIBrightMagenta
	ANSIBrightCyan
	ANSIBrightWhite
	ANSISlotCount = 16
)

// ThemeToken is a foreground or background theme token: upstream `ThemeToken = ThemeColor | ThemeBg` (theme.ts:117).
type ThemeToken string
