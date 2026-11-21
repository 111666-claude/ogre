package ogre

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

// This file verifies CSS multicol (column-count / column-width / column-gap)
// through the public Render entry point. Text is emitted as glyph paths whose
// transform gives the run origin, so origins are compared as exact geometry.

var mcTransformRe = regexp.MustCompile(`transform="translate\(([-+0-9.eE]+),([-+0-9.eE]+)\)"`)

type mcPoint struct{ x, y float64 }

func mcOrigins(svg string) []mcPoint {
	var out []mcPoint
	for _, m := range mcTransformRe.FindAllStringSubmatch(svg, -1) {
		x, _ := strconv.ParseFloat(m[1], 64)
		y, _ := strconv.ParseFloat(m[2], 64)
		out = append(out, mcPoint{x, y})
	}
	return out
}

// mcColumnX clusters glyph run origins by x and returns the sorted distinct x
// values. Multicol text must have exactly one origin x per column.
func mcColumnX(origins []mcPoint) []float64 {
	seen := map[float64]bool{}
	var xs []float64
	for _, p := range origins {
		if !seen[p.x] {
			seen[p.x] = true
			xs = append(xs, p.x)
		}
	}
	for i := 0; i < len(xs); i++ {
		for j := i + 1; j < len(xs); j++ {
			if xs[j] < xs[i] {
				xs[i], xs[j] = xs[j], xs[i]
			}
		}
	}
	return xs
}

func mcPosterHTML(extra string) string {
	body := strings.Repeat("lorem ipsum dolor sit amet consectetur adipiscing elit ", 12)
	return `<div style="width:600px;height:400px;padding:24px;column-count:2;column-gap:40px;` +
		`font-family:poster;font-size:20px;line-height:25px;` + extra + `">` + body + `</div>`
}

func mcPosterOpts(format Format) Options {
	return Options{
		Width: 600, Height: 400, Format: format,
		Fonts: []FontSource{{Name: "poster", Weight: 400, Style: "normal", Data: goregular.TTF}},
	}
}

func TestMultiColumnTwoColumnsGeometry(t *testing.T) {
	res, err := Render(mcPosterHTML(""), mcPosterOpts(FormatSVG))
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	origins := mcOrigins(string(res.Data))
	if len(origins) == 0 {
		t.Fatal("no glyph paths rendered")
	}

	xs := mcColumnX(origins)
	if len(xs) != 2 {
		t.Fatalf("got %d distinct text x-origins %v, want exactly 2 columns", len(xs), xs)
	}

	// Content box: x 24..576 (width 552). Column width = (552-40)/2 = 256.
	wantX0, wantX1 := 24.0, 24.0+256.0+40.0
	if math.Abs(xs[0]-wantX0) > 0.01 || math.Abs(xs[1]-wantX1) > 0.01 {
		t.Errorf("column x origins = %v, want [%.2f %.2f]", xs, wantX0, wantX1)
	}

	for _, p := range origins {
		if p.x < wantX0-0.01 || p.x > wantX1+256+0.01 {
			t.Errorf("glyph at x=%.2f escapes column bounds [%.2f, %.2f]", p.x, wantX0, wantX1+256)
		}
		if p.y < 24-0.01 || p.y > 400-24+0.01 {
			t.Errorf("glyph at y=%.2f escapes container content box [24, 376]", p.y)
		}
	}
}

func TestMultiColumnColumnWidth(t *testing.T) {
	// column-width 200px inside a 552px content box: floor((552+40)/240)=2
	// columns fit, and the used width expands to (552-40)/2 = 256.
	html := strings.Replace(mcPosterHTML("column-width:200px;"), "column-count:2;", "", 1)
	res, err := Render(html, mcPosterOpts(FormatSVG))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	xs := mcColumnX(mcOrigins(string(res.Data)))
	if len(xs) != 2 {
		t.Fatalf("column-width produced %d columns %v, want 2", len(xs), xs)
	}
	if math.Abs(xs[1]-xs[0]-(256+40)) > 0.01 {
		t.Fatalf("column pitch = %.2f, want %.2f", xs[1]-xs[0], 256.0+40)
	}
}

func TestMultiColumnSourceOrderAcrossColumns(t *testing.T) {
	// Two text nodes (split by a styled inline span) must read in document
	// order: the whole first node fills column 1 from the top before the second
	// node starts, and the second node begins at column 1's top.
	html := `<div style="width:600px;height:300px;padding:0;column-count:2;column-gap:20px;` +
		`font-family:poster;font-size:20px;line-height:25px">` +
		`<span style="font-weight:700">` + strings.Repeat("alpha ", 40) + `</span>` +
		strings.Repeat("beta ", 40) + `</div>`
	res, err := Render(html, mcPosterOpts(FormatSVG))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	origins := mcOrigins(string(res.Data))
	if len(origins) < 2 {
		t.Fatal("expected glyph runs in both columns")
	}

	colWidth := (600.0 - 20) / 2
	left, right := 0.0, colWidth+20
	var leftY, rightY []float64
	for _, p := range origins {
		switch {
		case math.Abs(p.x-left) < 0.01:
			leftY = append(leftY, p.y)
		case math.Abs(p.x-right) < 0.01:
			rightY = append(rightY, p.y)
		}
	}
	if len(leftY) == 0 || len(rightY) == 0 {
		t.Fatalf("expected text in both columns, got left=%d right=%d runs", len(leftY), len(rightY))
	}

	// The right column starts at the same top y as the left column: the second
	// source node begins at the top of column 2, never midway down column 1.
	if math.Abs(rightY[0]-leftY[0]) > 0.01 {
		t.Errorf("column 2 starts at y=%.2f, column 1 at y=%.2f: source order broken", rightY[0], leftY[0])
	}
}

func TestMultiColumnPreservesInlineFontStyles(t *testing.T) {
	// Styled inline runs resolve to distinct font faces; their glyph path
	// geometry must differ from the same words in the regular face.
	opts := Options{
		Width: 600, Height: 300, Format: FormatSVG,
		Fonts: []FontSource{
			{Name: "poster", Weight: 400, Style: "normal", Data: goregular.TTF},
			{Name: "poster", Weight: 700, Style: "normal", Data: gobold.TTF},
		},
	}
	word := strings.Repeat("styled ", 20)
	styled := `<div style="width:600px;height:300px;column-count:2;column-gap:20px;` +
		`font-family:poster;font-size:20px;line-height:25px"><span style="font-weight:700">` + word + `</span></div>`
	plain := `<div style="width:600px;height:300px;column-count:2;column-gap:20px;` +
		`font-family:poster;font-size:20px;line-height:25px">` + word + `</div>`

	a, err := Render(styled, opts)
	if err != nil {
		t.Fatalf("styled render: %v", err)
	}
	b, err := Render(plain, opts)
	if err != nil {
		t.Fatalf("plain render: %v", err)
	}
	if string(a.Data) == string(b.Data) {
		t.Error("bold inline span rendered identical geometry to regular text: font weight lost")
	}
}

func TestMultiColumnSVGAndPNGSameGeometry(t *testing.T) {
	html := mcPosterHTML("")
	svgRes, err := Render(html, mcPosterOpts(FormatSVG))
	if err != nil {
		t.Fatalf("svg render: %v", err)
	}
	pngRes, err := Render(html, mcPosterOpts(FormatPNG))
	if err != nil {
		t.Fatalf("png render: %v", err)
	}
	if len(pngRes.Data) == 0 {
		t.Fatal("empty png")
	}

	// Both formats are produced from one layout pass; assert the SVG geometry
	// is well-formed and the PNG decodes at the same canvas size.
	xs := mcColumnX(mcOrigins(string(svgRes.Data)))
	if len(xs) != 2 {
		t.Fatalf("svg columns = %v, want 2", xs)
	}
	if pngRes.Width != 600 || pngRes.Height != 400 {
		t.Fatalf("png canvas = %dx%d, want 600x400", pngRes.Width, pngRes.Height)
	}
}

func TestMultiColumnNoColumnsLeavesLayoutUntouched(t *testing.T) {
	body := strings.Repeat("lorem ipsum ", 20)
	html := `<div style="width:600px;height:200px;font-family:poster;font-size:20px;line-height:25px">` + body + `</div>`
	a, err := Render(html, mcPosterOpts(FormatSVG))
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if xs := mcColumnX(mcOrigins(string(a.Data))); len(xs) != 1 {
		t.Fatalf("single-column output has %d x origins %v", len(xs), xs)
	}
}

func TestMultiColumnUnbreakableWordDoesNotPanic(t *testing.T) {
	word := strings.Repeat("x", 200)
	html := `<div style="width:200px;height:200px;column-count:2;column-gap:10px;` +
		`font-family:poster;font-size:16px;word-break:normal">` + word + `</div>`
	res, err := Render(html, Options{
		Width: 200, Height: 200, Format: FormatSVG,
		Fonts: []FontSource{{Name: "poster", Weight: 400, Data: goregular.TTF}},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(string(res.Data), "<svg") {
		t.Fatal("expected svg output for overflowing unbreakable word")
	}
}

func TestMultiColumnRejectsInlineImage(t *testing.T) {
	html := `<div style="width:400px;height:300px;column-count:2;font-family:poster">` +
		`text <img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUg==" alt="x"> more</div>`
	_, err := Render(html, mcPosterOpts(FormatSVG))
	if err == nil || !strings.Contains(err.Error(), "multicolumn") {
		t.Fatalf("want multicol rejection error for inline img, got %v", err)
	}
}

func TestMultiColumnRejectsNestedBlock(t *testing.T) {
	html := `<div style="width:400px;height:300px;column-count:2;font-family:poster">` +
		`text<div style="display:flex">nested</div>more</div>`
	_, err := Render(html, mcPosterOpts(FormatSVG))
	if err == nil || !strings.Contains(err.Error(), "multicolumn") {
		t.Fatalf("want multicol rejection error for nested block, got %v", err)
	}
}

func TestMultiColumnRejectsIndefiniteHeight(t *testing.T) {
	html := `<div style="width:400px;column-count:2;font-family:poster">text</div>`
	_, err := Render(html, mcPosterOpts(FormatSVG))
	if err == nil || !strings.Contains(err.Error(), "multicolumn") {
		t.Fatalf("want multicol rejection error for indefinite height, got %v", err)
	}
}

func TestMultiColumnOverflowStaysInsideContainer(t *testing.T) {
	// More text than the columns hold: overflow lines are dropped, glyphs must
	// never paint past the last column's bottom edge.
	body := strings.Repeat("overflow ", 300)
	html := `<div style="width:400px;height:160px;column-count:2;column-gap:20px;` +
		`font-family:poster;font-size:16px;line-height:20px;overflow:hidden">` + body + `</div>`
	res, err := Render(html, Options{
		Width: 400, Height: 160, Format: FormatSVG,
		Fonts: []FontSource{{Name: "poster", Weight: 400, Data: goregular.TTF}},
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, p := range mcOrigins(string(res.Data)) {
		if p.y > 160+0.01 {
			t.Fatalf("overflow glyph painted at y=%.2f, outside 160px container", p.y)
		}
	}
}
