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
}
