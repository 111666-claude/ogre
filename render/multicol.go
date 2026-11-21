package render

import (
	"image/color"
	"strings"

	fontpkg "github.com/macawls/ogre/v3/font"
	"github.com/macawls/ogre/v3/layout"
	"github.com/macawls/ogre/v3/parse"
	"github.com/macawls/ogre/v3/style"
)

// columnRect returns the absolute content-box origin and size of a multicol
// column. Geometry in the layout tree is stored relative to the container's
// border box.
func columnRect(mc *layout.MultiColumnLayout, index int, containerX, containerY float64) (x, y, w, h float64) {
	col := mc.Columns[index]
	return containerX + col.X, containerY + col.Y, col.Width, col.Height
}

func (ctx *RenderContext) renderMultiColumnSVG(b *strings.Builder, mc *layout.MultiColumnLayout, containerX, containerY float64) {
	idGen := func(prefix string) string { return ctx.ids.next(prefix) }

	for _, pn := range mc.Order {
		cs := ctx.Styles[pn]
		lines, ok := ctx.WrappedText[pn]
		if !ok || len(lines) == 0 {
			continue
		}
		placement := mc.Placements[pn]
		if placement.Column >= len(mc.Columns) {
			continue
		}

		x, y, w, h := columnRect(mc, placement.Column, containerX, containerY)
		result := RenderTextFragment(lines, cs, placement.StartLine, placement.EndLine,
			x, y, w, h, idGen, ctx.FontMgr, ctx.EmojiProvider)
		b.WriteString(result.Shadows)
		b.WriteString(result.Content)
		b.WriteString(result.Decorations)
	}

	for _, c := range mc.Continuations {
		if c.Column >= len(mc.Columns) {
			continue
		}
		cs := ctx.Styles[c.ParseNode]
		lines, ok := ctx.WrappedText[c.ParseNode]
		if !ok || len(lines) == 0 {
			continue
		}
		x, y, w, h := columnRect(mc, c.Column, containerX, containerY)
		result := RenderTextFragment(lines, cs, c.StartLine, c.StartLine+c.EndLine,
			x, y, w, h, idGen, ctx.FontMgr, ctx.EmojiProvider)
		b.WriteString(result.Shadows)
		b.WriteString(result.Content)
		b.WriteString(result.Decorations)
	}
}

// renderMultiColumn paints the text fragments of a multicol container onto
// the raster image. The container's box, background, borders, and overflow
// clip were already painted by renderNodeContent.
func (r *PNGRenderer) renderMultiColumn(mc *layout.MultiColumnLayout, containerX, containerY float64) {
	drawFragment := func(textPN *parse.Node, start, end, colIndex int) {
		if colIndex >= len(mc.Columns) {
			return
		}
		cs := r.styles[textPN]
		lines, ok := r.wrappedText[textPN]
		if !ok || len(lines) == 0 || start < 0 || end > len(lines) || start >= end {
			return
		}
		r.drawTextFragment(lines[start:end], cs, mc.Columns[colIndex], containerX, containerY)
	}

	for _, textPN := range mc.Order {
		placement := mc.Placements[textPN]
		drawFragment(textPN, placement.StartLine, placement.EndLine, placement.Column)
	}
	for _, c := range mc.Continuations {
		drawFragment(c.ParseNode, c.StartLine, c.StartLine+c.EndLine, c.Column)
	}
}

func (r *PNGRenderer) drawTextFragment(lines []fontpkg.TextLine, cs *style.ComputedStyle, col layout.ColumnBox, containerX, containerY float64) {
	if r.fonts == nil {
		return
	}
	family := cs.FontFamily
	if family == "" {
		family = "default"
	}
	weight := cs.FontWeight
	if weight == 0 {
		weight = 400
	}
	fstyle := cs.FontStyle
	if fstyle == "" {
		fstyle = "normal"
	}
	face := r.fonts.Resolve(family, weight, fstyle)
	if face == nil {
		return
	}
	size := cs.FontSize
	if size == 0 {
		size = 16
	}
	ff, err := r.fonts.NewFace(face, size)
	if err != nil {
		return
	}
	lineHeight := cs.LineHeight
	if lineHeight == 0 {
		lineHeight = size * 1.2
	}
	ascent, descent := r.fonts.AscentDescent(face, size, toFontVariations(cs.FontVariationSettings))
	halfLeading := (lineHeight - (ascent + descent)) / 2

	boxX, boxY := containerX+col.X, containerY+col.Y
	tc := styleToColor(cs.Color)
	if cs.Color.A == -1 {
		tc = color.RGBA{0, 0, 0, 255}
	}

	for i, line := range lines {
		text := applyTextTransform(line.Text, cs.TextTransform)
		x := alignX(boxX, col.Width, line.Width, cs.TextAlign)
		y := boxY + halfLeading + ascent + float64(i)*lineHeight
		r.drawTextWithEmoji(text, x, y, ascent, size, tc, ff, cs)
	}
}
