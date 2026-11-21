package layout

import (
	"testing"

	"github.com/macawls/ogre/v3/parse"
	"github.com/macawls/ogre/v3/style"
)

func mcTestTree(t *testing.T, html string, containerCS *style.ComputedStyle) (*LayoutTree, *parse.Node, map[*parse.Node]*style.ComputedStyle) {
	t.Helper()
	root, err := parse.Parse(html)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	styles := map[*parse.Node]*style.ComputedStyle{}
	var walk func(n *parse.Node)
	walk = func(n *parse.Node) {
		if n == root {
			styles[n] = containerCS
		} else {
			cs := style.NewComputedStyle()
			cs.FontSize = 10
			cs.LineHeight = 10
			cs.ColumnWidth = style.Value{Unit: style.UnitNone}
			styles[n] = cs
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)

	tree := BuildTree(root, styles, func(pn *parse.Node, text string, cs *style.ComputedStyle, maxWidth float64) (float64, float64) {
		return maxWidth, 10
	})
	Compute(tree.Root, 400, 200)
	return tree, root, styles
}

func mcWrapLines(n int) WrapTextFunc {
	return func(pn *parse.Node, cs *style.ComputedStyle, columnWidth float64) []WrappedLine {
		lines := make([]WrappedLine, n)
		for i := range lines {
			lines[i] = WrappedLine{Text: "x", Width: 5}
		}
		return lines
	}
}

func mcContainerCS() *style.ComputedStyle {
	cs := style.NewComputedStyle()
	cs.ColumnCount = 2
	cs.Height = style.Value{Unit: style.UnitPx, Raw: 100}
	cs.LineHeight = 10
	cs.FontSize = 10
	return cs
}

func TestMultiColumnBalancesAcrossColumns(t *testing.T) {
	tree, container, styles := mcTestTree(t, `<div><span>one</span><span>two</span></div>`, mcContainerCS())

	wrap := func(pn *parse.Node, c *style.ComputedStyle, w float64) []WrappedLine {
		return mcWrapLines(6)(pn, c, w)
	}
	if err := ComputeMultiColumns(tree, styles, wrap); err != nil {
		t.Fatalf("multicolumn: %v", err)
	}

	mc := tree.MultiColumns[container]
	if len(mc.Columns) != 2 {
		t.Fatalf("got %d columns, want 2", len(mc.Columns))
	}
	if mc.OverflowLines != 0 {
		t.Errorf("overflow = %d, want 0", mc.OverflowLines)
	}

	// Two nodes, 6 lines each, column capacity 10. The first fills column 0
	// lines [0,6); the second takes [6,10) then continues in column 1.
	var first, second *parse.Node
	for n := range mc.Placements {
		if n.Text == "one" {
			first = n
		} else {
			second = n
		}
	}
	p1 := mc.Placements[first]
	if p1.Column != 0 || p1.StartLine != 0 || p1.EndLine != 6 {
		t.Errorf("first placement = %+v, want col 0 lines [0,6)", p1)
	}
	p2 := mc.Placements[second]
	if p2.Column != 0 || p2.StartLine != 6 || p2.EndLine != 10 {
		t.Errorf("second placement = %+v, want col 0 lines [6,10)", p2)
	}
	if len(mc.Continuations) != 1 {
		t.Fatalf("got %d continuations, want 1", len(mc.Continuations))
	}
	c := mc.Continuations[0]
	if c.ParseNode != second || c.Column != 1 || c.StartLine != 4 || c.EndLine != 2 {
		t.Errorf("continuation = %+v, want second node col 1, source line 4, 2 lines", c)
	}
}

func TestMultiColumnOverflowCounted(t *testing.T) {
	cs := mcContainerCS()
	cs.Height = style.Value{Unit: style.UnitPx, Raw: 50}

	tree, container, styles := mcTestTree(t, `<div>only</div>`, cs)
	if err := ComputeMultiColumns(tree, styles, mcWrapLines(15)); err != nil {
		t.Fatalf("multicolumn: %v", err)
	}
	mc := tree.MultiColumns[container]
	// Capacity 5 per column x 2 columns = 10 slots; 15 lines -> 5 overflow.
	if mc.OverflowLines != 5 {
		t.Errorf("overflow = %d, want 5", mc.OverflowLines)
	}
}

func TestMultiColumnRejectsNonPhrasing(t *testing.T) {
	tree, _, styles := mcTestTree(t, `<div>a<img src="x.png"/>b</div>`, mcContainerCS())
	err := ComputeMultiColumns(tree, styles, mcWrapLines(1))
	if err == nil {
		t.Fatal("want rejection for inline img")
	}
}

func TestMultiColumnRejectsIndefiniteHeight(t *testing.T) {
	cs := mcContainerCS()
	cs.Height = style.Value{Unit: style.UnitNone}

	tree, _, styles := mcTestTree(t, `<div>text</div>`, cs)
	err := ComputeMultiColumns(tree, styles, mcWrapLines(1))
	if err == nil {
		t.Fatal("want rejection for indefinite height")
	}
}

func TestResolveColumns(t *testing.T) {
	cases := []struct {
		name            string
		count           int
		width           float64
		hasWidth        bool
		contentWidth    float64
		gap             float64
		wantCount       int
		wantColumnWidth float64
	}{
		{"count only", 2, 0, false, 100, 20, 2, 40},
		{"width fits two", 0, 40, true, 100, 20, 2, 40},
		{"both, count wins when room", 2, 30, true, 100, 20, 2, 40},
		{"both, width limits count", 3, 40, true, 100, 20, 2, 40},
		{"width too narrow", 0, 120, true, 100, 20, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cs := style.NewComputedStyle()
			cs.ColumnCount = tc.count
			if tc.hasWidth {
				cs.ColumnWidth = style.Value{Unit: style.UnitPx, Raw: tc.width}
			}
			count, width := resolveColumns(cs, tc.contentWidth, tc.gap)
			if count != tc.wantCount || width != tc.wantColumnWidth {
				t.Errorf("got (%d, %.2f), want (%d, %.2f)", count, width, tc.wantCount, tc.wantColumnWidth)
			}
		})
	}
}
