package layout

import (
	"fmt"
	"math"

	"github.com/macawls/ogre/v3/parse"
	"github.com/macawls/ogre/v3/style"
)

// ErrMultiColumnUnsupported reports a multicol container whose content cannot
// be fragmented: an indefinite height, non-phrasing descendants, or a width
// that fits no column. Only runs of inline text (optionally styled with inline
// elements) participate in column fragmentation.
type ErrMultiColumnUnsupported struct {
	Reason string
}

func (e *ErrMultiColumnUnsupported) Error() string {
	return "multicolumn layout: " + e.Reason
}

type textSegment struct {
	pn *parse.Node
	cs *style.ComputedStyle
}

// phrasingTags are the inline elements whose text may be fragmented across
// columns. Everything else (media, tables, nested block boxes) is rejected.
var phrasingTags = map[string]bool{
	"a": true, "abbr": true, "b": true, "bdi": true, "bdo": true,
	"cite": true, "code": true, "data": true, "dfn": true, "em": true,
	"i": true, "kbd": true, "mark": true, "q": true, "rp": true,
	"rt": true, "ruby": true, "s": true, "samp": true, "small": true,
	"span": true, "strong": true, "sub": true, "sup": true,
	"time": true, "u": true, "var": true, "wbr": true,
}

// ComputeMultiColumns fragments every node whose computed style requests CSS
// multicol (column-count or column-width). Flex layout runs first so the
// containers have definite boxes; this pass then rewraps their text at the
// resolved column width and records per-node placement for the renderers.
func ComputeMultiColumns(tree *LayoutTree, styles map[*parse.Node]*style.ComputedStyle, wrap WrapTextFunc) error {
	for pn, ln := range tree.NodeMap {
		cs := styles[pn]
		if cs == nil {
			continue
		}
		hasWidth := cs.ColumnWidth.Unit == style.UnitPx && cs.ColumnWidth.Raw > 0
		if cs.ColumnCount <= 0 && !hasWidth {
			continue
		}
		mc, err := layoutMultiColumn(pn, ln, cs, styles, wrap)
		if err != nil {
			return err
		}
		tree.MultiColumns[pn] = mc
		for textPN := range mc.Placements {
			tree.MultiColumnOwners[textPN] = pn
		}
		for _, c := range mc.Continuations {
			tree.MultiColumnOwners[c.ParseNode] = pn
		}
	}
	return nil
}

func layoutMultiColumn(pn *parse.Node, ln *Node, cs *style.ComputedStyle, styles map[*parse.Node]*style.ComputedStyle, wrap WrapTextFunc) (*MultiColumnLayout, error) {
	if !ln.Style.Height.IsDefined() {
		return nil, &ErrMultiColumnUnsupported{Reason: "column-count/column-width requires a fixed-height container"}
	}

	segments, err := flattenInline(pn, styles)
	if err != nil {
		return nil, err
	}

	padL := ln.Layout.Padding[3] + ln.Layout.Border[3]
	padR := ln.Layout.Padding[1] + ln.Layout.Border[1]
	padT := ln.Layout.Padding[0] + ln.Layout.Border[0]
	padB := ln.Layout.Padding[2] + ln.Layout.Border[2]
	contentWidth := ln.Layout.Width - padL - padR
	contentHeight := ln.Layout.Height - padT - padB
	if contentWidth <= 0 || contentHeight <= 0 {
		return nil, &ErrMultiColumnUnsupported{Reason: "container content box has zero size"}
	}

	gap := cs.ColumnGap
	count, colWidth := resolveColumns(cs, contentWidth, gap)
	if count <= 0 || colWidth <= 0 {
		return nil, &ErrMultiColumnUnsupported{Reason: "no column fits inside the container width"}
	}

	columns := make([]ColumnBox, count)
	for i := range columns {
		columns[i] = ColumnBox{
			X:      padL + float64(i)*(colWidth+gap),
			Y:      padT,
			Width:  colWidth,
			Height: contentHeight,
		}
	}

	mc := &MultiColumnLayout{
		Columns:    columns,
		Placements: make(map[*parse.Node]TextPlacement),
	}

	capacity := maxLinesPerColumn(cs, contentHeight)
	cursor := 0

	for _, seg := range segments {
		lines := wrap(seg.pn, seg.cs, colWidth)
		n := len(lines)
		if n == 0 {
			continue
		}

		firstColumn, firstStart := cursor/capacity, cursor%capacity
		mc.Placements[seg.pn] = TextPlacement{
			Column:    firstColumn,
			StartLine: firstStart,
			EndLine:   min(capacity, firstStart+n),
		}
		mc.Order = append(mc.Order, seg.pn)

		for pos := capacity - firstStart; pos < n; pos += capacity {
			col := firstColumn + (firstStart+pos)/capacity
			if col >= count {
				break
			}
			mc.Continuations = append(mc.Continuations, Continuation{
				ParseNode: seg.pn,
				Column:    col,
				StartLine: pos,
				EndLine:   min(capacity, n-pos),
			})
		}

		cursor += n
	}

	if total := capacity * count; cursor > total {
		mc.OverflowLines = cursor - total
	}

	return mc, nil
}

// resolveColumns applies CSS column-count/column-width resolution against the
// content width and returns the effective count and per-column width.
func resolveColumns(cs *style.ComputedStyle, contentWidth, gap float64) (int, float64) {
	hasWidth := cs.ColumnWidth.Unit == style.UnitPx && cs.ColumnWidth.Raw > 0
	count := cs.ColumnCount

	switch {
	case count > 0 && hasWidth:
		width := cs.ColumnWidth.Raw
		fit := int((contentWidth + gap) / (width + gap))
		if fit < 1 {
			return 0, 0
		}
		if fit < count {
			count = fit
		} else {
			width = (contentWidth - gap*float64(count-1)) / float64(count)
		}
		return count, width
	case hasWidth:
		width := cs.ColumnWidth.Raw
		fit := int((contentWidth + gap) / (width + gap))
		if fit < 1 {
			return 0, 0
		}
		width = (contentWidth - gap*float64(fit-1)) / float64(fit)
		return fit, width
	default:
		if count <= 0 {
			return 0, 0
		}
		width := (contentWidth - gap*float64(count-1)) / float64(count)
		if width <= 0 {
			return 0, 0
		}
		return count, width
	}
}

// maxLinesPerColumn is the whole number of line boxes that fit the column
// height given the container's line height.
func maxLinesPerColumn(cs *style.ComputedStyle, columnHeight float64) int {
	lh := cs.LineHeight
	if lh <= 0 {
		lh = 1.2 * cs.FontSize
	}
	if lh <= 0 {
		return 1
	}
	n := int(math.Floor(columnHeight / lh))
	if n < 1 {
		n = 1
	}
	return n
}

// flattenInline walks the subtree of a multicol container and returns its text
// nodes in document order. Content that cannot be fragmented as phrasing
// content rejects the whole container.
func flattenInline(pn *parse.Node, styles map[*parse.Node]*style.ComputedStyle) ([]textSegment, error) {
	var segments []textSegment
	var walk func(n *parse.Node, depth int) error
	walk = func(n *parse.Node, depth int) error {
		switch n.Type {
		case parse.TextNode:
			segments = append(segments, textSegment{pn: n, cs: styles[n]})
			return nil
		case parse.ElementNode:
			cs := styles[n]
			if cs != nil && cs.Display == style.DisplayNone {
				return nil
			}
			if depth > 0 {
				if cs != nil && cs.Position == style.PositionAbsolute {
					return &ErrMultiColumnUnsupported{Reason: fmt.Sprintf("<%s> is absolutely positioned", n.Tag)}
				}
				if !phrasingTags[n.Tag] {
					return &ErrMultiColumnUnsupported{Reason: fmt.Sprintf("<%s> is not phrasing content; nested blocks and inline media are not supported inside columns", n.Tag)}
				}
			}
			for _, child := range n.Children {
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(pn, 0); err != nil {
		return nil, err
	}
	return segments, nil
}
