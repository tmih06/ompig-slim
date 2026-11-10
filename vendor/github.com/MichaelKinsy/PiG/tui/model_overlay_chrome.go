package tui

import (
	"fmt"
	"strings"

	"github.com/MichaelKinsy/PiG/internal/modelroles"
	"github.com/MichaelKinsy/PiG/tui/widthx"
)

// modelOverlayTop translates OMP overlay-box.topBorder and topBorderSplit.
// The split tee sits over the body divider (sidebar content width + 3), not
// over the end of the sidebar text. Titles clip before they can displace it.
func modelOverlayTop(width int, title string, sidebar int, warning bool) string {
	th := modelOverlayTheme()
	color := th.Border
	if warning {
		color = th.Error
	}
	left := max(0, width-2)
	if sidebar >= 0 {
		left = sidebar + 2
	}
	shown := widthx.TruncateToWidth(" "+title+" ", max(0, left-1), "…", false)
	titleColor := th.Accent
	if warning {
		titleColor = color
	}
	line := fg(color, "╭─") + th.Bold(fg(titleColor, shown)) + fg(color, strings.Repeat("─", max(0, left-1-widthx.VisibleWidth(shown))))
	if sidebar >= 0 {
		line += fg(color, "┬"+strings.Repeat("─", max(0, width-sidebar-5)))
	}
	return widthx.TruncateToWidth(line+fg(color, "╮"), width, "", false)
}

// modelOverlayRow pads ANSI-styled content to the full inset before drawing
// either vertical. Short status/detail/footer lines must never shrink the box.
func modelOverlayRow(content string, width int, warning bool) string {
	color := modelOverlayTheme().Border
	if warning {
		color = modelOverlayTheme().Error
	}
	return fg(color, "│") + " " + widthx.TruncateToWidth(content, max(0, width-4), "…", true) + " " + fg(color, "│")
}

// modelSegmentPalette resolves OMP's positional segment palette. Distinct resolved colors, rather than token names,
// decide palette membership so monochrome themes do not produce fake variety.
func modelSegmentPalette(count int) []string {
	th := modelOverlayTheme()
	palette := []string{}
	seen := map[string]bool{}
	for _, token := range []string{"accent", "success", "warning", "error", "mdCode", "mdLink", "syntaxString", "syntaxKeyword", "syntaxFunction", "syntaxNumber", "syntaxOperator", "syntaxVariable"} {
		color := th.GetFgAnsi(token)
		if color != "" && !seen[color] {
			seen[color] = true
			palette = append(palette, color)
			if len(palette) >= count {
				break
			}
		}
	}
	if len(palette) == 0 {
		palette = []string{th.GetFgAnsi("accent")}
	}
	return palette
}

// modelRoleColor resolves a role to the SGR sequence of its own catalog color, the
// same token RoleInfo reports to the model manager's role rows. An unknown token,
// or one the theme leaves undefined, falls back to muted: RoleInfo inherits a color
// from user-authored modelTags, and GetFgAnsi panics on a token the theme does not
// define. Roles that share a token therefore draw alike — plan, task and judge are
// all muted — which is the point: one role, one color.
func modelRoleColor(config modelroles.Config, role string) string {
	th := modelOverlayTheme()
	if token := config.RoleInfo(role).Color; th.HasFgToken(token) {
		return th.GetFgAnsi(token)
	}
	return th.GetFgAnsi("muted")
}

// modelRoleTrack paints a track whose segments are roles, each in its own catalog
// color instead of the positional palette. OMP colors its cycle track by position;
// this colors it by role, so a role keeps one hue when the cycle is reordered.
func modelRoleTrack(config modelroles.Config, roles []string, active int) (string, [][2]int) {
	labels := make([]string, len(roles))
	colors := make([]string, len(roles))
	for i, role := range roles {
		labels[i] = SanitizeModelText(role)
		colors[i] = modelRoleColor(config, role)
	}
	return modelSegmentTrackColors(labels, colors, active)
}

// modelSegmentTrack paints a non-role track (the kind and role tab strips) with
// OMP's positional palette and returns display-column chip ranges so mouse
// activation and visible colored caps use identical geometry.
func modelSegmentTrack(labels []string, active int) (string, [][2]int) {
	return modelSegmentTrackColors(labels, modelSegmentPalette(len(labels)), active)
}

// modelSegmentTrackColors is the shared painter. colors supplies one SGR opening
// sequence per segment and is indexed modulo its length, so the positional palette
// may be shorter than the track. The active segment turns its color into a
// background chip and picks a black or white label for contrast.
func modelSegmentTrackColors(labels, colors []string, active int) (string, [][2]int) {
	th := modelOverlayTheme()
	out := ""
	ranges := make([][2]int, len(labels))
	for i, label := range labels {
		if i > 0 {
			if i == active || i-1 == active {
				out += "  "
			} else {
				out += " " + fg(th.Border, "┆") + " "
			}
		}
		ranges[i][0] = widthx.VisibleWidth(out)
		color := colors[i%len(colors)]
		if i != active {
			out += color + label + SGRFgReset
		} else {
			contrast := th.GetFgAnsi("text")
			var r, g, b int
			if n, _ := fmt.Sscanf(color, "\x1b[38;2;%d;%d;%dm", &r, &g, &b); n == 3 {
				contrast = "\x1b[38;2;255;255;255m"
				if .299*float64(r)+.587*float64(g)+.114*float64(b) > 140 {
					contrast = "\x1b[38;2;0;0;0m"
				}
			}
			out += color + "◀" + strings.Replace(color, "[38;", "[48;", 1) + contrast + "\x1b[1m " + label + " \x1b[22m\x1b[49m" + color + "▶" + SGRFgReset
		}
		ranges[i][1] = widthx.VisibleWidth(out)
	}
	return out, ranges
}

// modelOverlayDivider closes the split column with a tee-up, matching OMP's
// overlay-box.dividerSplit; sidebar is its content width, excluding insets.
func modelOverlayDivider(width, sidebar int) string {
	return fg(modelOverlayTheme().Border, "├"+strings.Repeat("─", max(0, sidebar+2))+"┴"+strings.Repeat("─", max(0, width-sidebar-5))+"┤")
}
