package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/ekosup/d8s/internal/resource"
)

// Colours used across the UI. Skins replace this in a later milestone.
var (
	colorHeader   = tcell.ColorWhite
	colorBorder   = tcell.ColorSteelBlue
	colorTitle    = tcell.ColorAqua
	colorSelectBg = tcell.ColorAqua
	colorSelectFg = tcell.ColorBlack
	colorMark     = tcell.ColorDarkOrange

	toneColors = map[resource.Tone]tcell.Color{
		resource.ToneNormal: tcell.ColorLightSkyBlue,
		resource.ToneGood:   tcell.ColorLightGreen,
		resource.ToneWarn:   tcell.ColorYellow,
		resource.ToneBad:    tcell.ColorOrangeRed,
		resource.ToneMuted:  tcell.ColorGray,
	}
)

func init() {
	// Use the terminal's own background instead of tview's black.
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	tview.Styles.ContrastBackgroundColor = tcell.ColorDefault
	tview.Styles.BorderColor = colorBorder
	tview.Styles.TitleColor = colorTitle

	// One border style whether focused or not; tview doubles it by default.
	tview.Borders.HorizontalFocus = tview.Borders.Horizontal
	tview.Borders.VerticalFocus = tview.Borders.Vertical
	tview.Borders.TopLeftFocus = tview.Borders.TopLeft
	tview.Borders.TopRightFocus = tview.Borders.TopRight
	tview.Borders.BottomLeftFocus = tview.Borders.BottomLeft
	tview.Borders.BottomRightFocus = tview.Borders.BottomRight
}
