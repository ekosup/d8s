package ui

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/ekosup/d8s/internal/resource"
)

// toneStyle is how rows of one tone are drawn.
type toneStyle struct {
	color tcell.Color
	attrs tcell.AttrMask
}

// skin is every colour and text attribute the UI uses. Views read the
// current one from theme; nothing else in the package names a colour.
type skin struct {
	text     tcell.Color // ordinary text in text views
	header   toneStyle   // table column headers
	border   tcell.Color
	title    tcell.Color
	selected tcell.Style // the highlighted table row
	mark     toneStyle   // rows marked for a bulk action
	dialog   tcell.Color // border of dialogs that ask something
	input    tcell.Style // text typed into a field
	label    tcell.Style // label in front of a field
	tones    map[resource.Tone]toneStyle

	// Tags for text with inline styling: "[colour:background:attributes]".
	tagKey, tagLabel, tagValue string // header and help: keys, field names, values
	tagMuted                   string // suggestions, inactive breadcrumbs
	tagCurrent                 string // the current breadcrumb
	tagInfo, tagWarn, tagError string // status messages
	tagMatch                   string // a search hit in a pager
}

var skins = map[string]skin{
	"dark": {
		text:     tcell.ColorDefault,
		header:   toneStyle{tcell.ColorWhite, tcell.AttrBold},
		border:   tcell.ColorSteelBlue,
		title:    tcell.ColorAqua,
		selected: tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorAqua),
		mark:     toneStyle{tcell.ColorDarkOrange, tcell.AttrBold},
		dialog:   tcell.ColorYellow,
		input:    tcell.StyleDefault.Foreground(tcell.ColorWhite),
		label:    tcell.StyleDefault.Foreground(tcell.ColorAqua).Bold(true),
		tones: map[resource.Tone]toneStyle{
			resource.ToneNormal: {tcell.ColorLightSkyBlue, 0},
			resource.ToneGood:   {tcell.ColorLightGreen, 0},
			resource.ToneWarn:   {tcell.ColorYellow, 0},
			resource.ToneBad:    {tcell.ColorOrangeRed, 0},
			resource.ToneMuted:  {tcell.ColorGray, 0},
		},
		tagKey: "[dodgerblue::b]", tagLabel: "[aqua]", tagValue: "[white::b]",
		tagMuted: "[gray]", tagCurrent: "[aqua]",
		tagInfo: "[white]", tagWarn: "[yellow]", tagError: "[orangered]",
		tagMatch: "[black:yellow]",
	},
	// For terminals with a light background: nothing pale on the default
	// background, where it would disappear.
	"light": {
		text:     tcell.ColorDefault,
		header:   toneStyle{tcell.ColorDefault, tcell.AttrBold},
		border:   tcell.ColorBlue,
		title:    tcell.ColorNavy,
		selected: tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.ColorNavy),
		mark:     toneStyle{tcell.ColorPurple, tcell.AttrBold},
		dialog:   tcell.ColorDarkGoldenrod,
		input:    tcell.StyleDefault.Foreground(tcell.ColorDefault),
		label:    tcell.StyleDefault.Foreground(tcell.ColorNavy).Bold(true),
		tones: map[resource.Tone]toneStyle{
			resource.ToneNormal: {tcell.ColorDefault, 0},
			resource.ToneGood:   {tcell.ColorDarkGreen, 0},
			resource.ToneWarn:   {tcell.ColorDarkGoldenrod, 0},
			resource.ToneBad:    {tcell.ColorRed, 0},
			resource.ToneMuted:  {tcell.ColorGray, 0},
		},
		tagKey: "[blue::b]", tagLabel: "[teal]", tagValue: "[-::b]",
		tagMuted: "[gray]", tagCurrent: "[navy::b]",
		tagInfo: "[-]", tagWarn: "[darkgoldenrod]", tagError: "[red]",
		tagMatch: "[white:navy]",
	},
	// No colour at all: for NO_COLOR, dumb terminals and screenshots in
	// print. What colour said, bold, underline, dim and reverse say.
	"mono": {
		text:     tcell.ColorDefault,
		header:   toneStyle{tcell.ColorDefault, tcell.AttrBold},
		border:   tcell.ColorDefault,
		title:    tcell.ColorDefault,
		selected: tcell.StyleDefault.Reverse(true),
		mark:     toneStyle{tcell.ColorDefault, tcell.AttrBold | tcell.AttrUnderline},
		dialog:   tcell.ColorDefault,
		input:    tcell.StyleDefault.Underline(true),
		label:    tcell.StyleDefault.Bold(true),
		tones: map[resource.Tone]toneStyle{
			resource.ToneNormal: {tcell.ColorDefault, 0},
			resource.ToneGood:   {tcell.ColorDefault, tcell.AttrItalic},
			resource.ToneWarn:   {tcell.ColorDefault, tcell.AttrUnderline},
			resource.ToneBad:    {tcell.ColorDefault, tcell.AttrBold},
			resource.ToneMuted:  {tcell.ColorDefault, tcell.AttrDim},
		},
		tagKey: "[::b]", tagLabel: "[-]", tagValue: "[::b]",
		tagMuted: "[::d]", tagCurrent: "[::b]",
		tagInfo: "[-]", tagWarn: "[::u]", tagError: "[::b]",
		tagMatch: "[::r]",
	},
}

// theme is the skin in use.
var theme skin

func init() {
	if err := SetSkin("dark"); err != nil {
		panic(err)
	}
}

// SetSkin selects a skin by name. Call it before building the App: widgets
// take their colours when they are created.
func SetSkin(name string) error {
	s, ok := skins[name]
	if !ok {
		return fmt.Errorf("unknown skin %q", name)
	}
	theme = s

	// The terminal's own background, not tview's black.
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	tview.Styles.ContrastBackgroundColor = tcell.ColorDefault
	tview.Styles.MoreContrastBackgroundColor = tcell.ColorDefault
	tview.Styles.PrimaryTextColor = s.text
	tview.Styles.SecondaryTextColor = s.text
	tview.Styles.TertiaryTextColor = s.text
	tview.Styles.InverseTextColor = s.text
	tview.Styles.ContrastSecondaryTextColor = s.text
	tview.Styles.BorderColor = s.border
	tview.Styles.TitleColor = s.title
	tview.Styles.GraphicsColor = s.border

	// One border style whether focused or not; tview doubles it by default.
	tview.Borders.HorizontalFocus = tview.Borders.Horizontal
	tview.Borders.VerticalFocus = tview.Borders.Vertical
	tview.Borders.TopLeftFocus = tview.Borders.TopLeft
	tview.Borders.TopRightFocus = tview.Borders.TopRight
	tview.Borders.BottomLeftFocus = tview.Borders.BottomLeft
	tview.Borders.BottomRightFocus = tview.Borders.BottomRight
	return nil
}

// SkinFor decides which skin to use: the configured one, unless the
// environment asks for no colour (the NO_COLOR convention, or a terminal
// that cannot show any).
func SkinFor(configured string, getenv func(string) string) string {
	if getenv("NO_COLOR") != "" || getenv("TERM") == "dumb" {
		return "mono"
	}
	return configured
}
