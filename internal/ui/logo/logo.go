// Package logo renders a Token Harbor / thcode wordmark in a stylized way.
//
// Forked from charmbracelet/crush — replaced the Crush C-R-U-S-H ASCII
// letterform wordmark with a simple bold gradient "thcode" title and
// "Token Harbor" subtitle, framed by the same diagonal-slash fields.
package logo

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/charmbracelet/x/ansi"
)

const diag = `╱`

// Opts are the options for rendering the thcode title art.
type Opts struct {
	FieldColor   color.Color // diagonal lines
	TitleColorA  color.Color // left gradient ramp point
	TitleColorB  color.Color // right gradient ramp point
	CharmColor   color.Color // brand text color (was Charm™, now "Token Harbor")
	VersionColor color.Color // version text color
	Width        int         // width of the rendered logo, used for truncation

	// Hyper is preserved for upstream-API parity. In the Token Harbor
	// distribution there is no separate Hyper-prefixed variant — the
	// flag is ignored.
	Hyper    bool
	Unstable bool // unused — preserved for upstream-API parity
}

// Render renders the THcoder logo. The compact argument determines
// whether it renders compact for the sidebar or wider for the main pane.
func Render(base lipgloss.Style, version string, compact bool, o Opts) string {
	const brand = "Token Harbor"
	const wordmark = "THcoder"

	fg := func(c color.Color, s string) string {
		return lipgloss.NewStyle().Foreground(c).Render(s)
	}

	// Render the wordmark as a bold gradient (single line). Width
	// matches the visible width of the rendered string so the framing
	// diagonals line up.
	title := styles.ApplyBoldForegroundGrad(base, wordmark, o.TitleColorA, o.TitleColorB)
	titleWidth := lipgloss.Width(title)

	// Meta row: brand on the left, version on the right, with the brand
	// reserved width so the row never overlaps the version.
	metaRowGap := 1
	maxVersionWidth := max(0, titleWidth-lipgloss.Width(brand)-metaRowGap)
	version = ansi.Truncate(version, maxVersionWidth, "…")
	gap := max(0, titleWidth-lipgloss.Width(brand)-lipgloss.Width(version))
	metaRow := fg(o.CharmColor, brand) + strings.Repeat(" ", gap) + fg(o.VersionColor, version)

	body := strings.TrimSpace(metaRow + "\n" + title)

	// Compact (sidebar) version: diagonal fields above + below.
	if compact {
		field := fg(o.FieldColor, strings.Repeat(diag, titleWidth))
		return strings.Join([]string{field, field, body, field, ""}, "\n")
	}

	bodyHeight := lipgloss.Height(body)

	// Left field.
	const leftWidth = 6
	leftFieldRow := fg(o.FieldColor, strings.Repeat(diag, leftWidth))
	leftField := new(strings.Builder)
	for range bodyHeight {
		fmt.Fprintln(leftField, leftFieldRow)
	}

	// Right field — diminishes by 1 char per row for a stair-step look.
	rightWidth := max(15, o.Width-titleWidth-leftWidth-2)
	const stepDownAt = 0
	rightField := new(strings.Builder)
	for i := range bodyHeight {
		width := rightWidth
		if i >= stepDownAt {
			width = rightWidth - (i - stepDownAt)
		}
		fmt.Fprint(rightField, fg(o.FieldColor, strings.Repeat(diag, width)), "\n")
	}

	const hGap = " "
	logo := lipgloss.JoinHorizontal(lipgloss.Top, leftField.String(), hGap, body, hGap, rightField.String())
	if o.Width > 0 {
		lines := strings.Split(logo, "\n")
		for i, line := range lines {
			lines[i] = ansi.Truncate(line, o.Width, "")
		}
		logo = strings.Join(lines, "\n")
	}
	return logo
}

// SmallRender renders a smaller, single-line version of the THcoder
// logo, suitable for sidebars or narrow windows.
func SmallRender(t *styles.Styles, width int, o Opts) string {
	const brand = "Token Harbor"
	const wordmark = "THcoder"
	title := t.Logo.SmallCharm.Render(brand)
	title = fmt.Sprintf("%s %s", title, styles.ApplyBoldForegroundGrad(t.Logo.GradCanvas, wordmark, t.Logo.SmallGradFromColor, t.Logo.SmallGradToColor))
	remainingWidth := width - lipgloss.Width(title) - 1
	if remainingWidth > 0 {
		lines := strings.Repeat("╱", remainingWidth)
		title = fmt.Sprintf("%s %s", title, t.Logo.SmallDiagonals.Render(lines))
	}
	return title
}
