package font

import (
	"os"
	"testing"
)

// TestMultiColumnWrapAtColumnWidth uses the repository's variable-font fixture
// to confirm wrapping at a multicol column width obeys the same line-breaking
// rules as ordinary text boxes. The layout engine rewraps every fragmented
// text node through WrapText at the resolved column width, so this pins the
// font-side behavior that column geometry depends on.
func TestMultiColumnWrapAtColumnWidth(t *testing.T) {
	data, err := os.ReadFile("testdata/Estedad-VF.ttf")
	if err != nil {
		t.Skipf("font fixture unavailable: %v", err)
	}
	mgr := NewManager()
	if err := mgr.LoadFont(FontSource{Name: "Estedad", Weight: 400, Data: data}); err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	face := mgr.Resolve("Estedad", 400, "normal")
	if face == nil {
		t.Fatal("fixture face not resolved")
	}
	ff, err := mgr.NewFace(face, 20)
	if err != nil {
		t.Fatalf("new face: %v", err)
	}

	const columnWidth = 180
	cfg := WrapConfig{
		MaxWidth:   columnWidth,
		FontFace:   ff,
		FontSize:   20,
		LineHeight: 28,
		Face:       face,
	}

	text := "column fragmentation must reuse the standard wrapping pipeline for every line"
	lines := WrapText(text, cfg)
	if len(lines) < 2 {
		t.Fatalf("expected wrapped output at width %d, got %d line", columnWidth, len(lines))
	}
	for i, line := range lines {
		if line.Width > columnWidth+0.01 {
			t.Errorf("line %d width %.2f exceeds column width %d", i, line.Width, columnWidth)
		}
		if line.Y != float64(i)*28 {
			t.Errorf("line %d Y = %.2f, want %.2f", i, line.Y, float64(i)*28)
		}
	}
}
