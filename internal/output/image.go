package output

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"strings"

	"github.com/a7g3/g3a/internal/engine"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var (
	colorHeaderBg     = color.RGBA{R: 225, G: 25, B: 49, A: 255}   // Telkomsel Primary Red (#E11931)
	colorHeaderSubBg  = color.RGBA{R: 152, G: 18, B: 35, A: 255}   // Telkomsel Deep Burgundy Sub (#981223)
	colorHeaderText   = color.RGBA{R: 255, G: 255, B: 255, A: 255} // Pure White
	colorRowBgEven    = color.RGBA{R: 255, G: 255, B: 255, A: 255} // Pure White
	colorRowBgOdd     = color.RGBA{R: 253, G: 246, B: 247, A: 255} // Subtle Rose/Red-tinted Row
	colorBorder       = color.RGBA{R: 229, G: 231, B: 235, A: 255} // Clean border gridline (#E5E7EB)
	colorHeaderBorder = color.RGBA{R: 255, G: 255, B: 255, A: 140} // Crisp Header separator
	colorCellText     = color.RGBA{R: 31, G: 41, B: 55, A: 255}    // Dark Slate text (#1F2937)
	colorFooterBg     = color.RGBA{R: 248, G: 249, B: 251, A: 255} // Subtle Footer bar
	colorFooterText   = color.RGBA{R: 107, G: 114, B: 128, A: 255} // Muted text (#6B7280)
	colorCardBg       = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	colorOuterBg      = color.RGBA{R: 243, G: 244, B: 246, A: 255} // Modern Neutral Canvas (#F3F4F6)
)

// RenderPNG renders the result as an Excel-styled table image (PNG).
func RenderPNG(w io.Writer, result *engine.Result) error {
	img, err := DrawTableImage(result)
	if err != nil {
		return err
	}
	return png.Encode(w, img)
}

// DrawTableImage generates an *image.RGBA representing the result table.
func DrawTableImage(result *engine.Result) (*image.RGBA, error) {
	regFont, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse regular font: %w", err)
	}
	boldFont, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse bold font: %w", err)
	}

	const dpi = 144.0 // High DPI / Retina for crystal clear text on mobile
	fontScale := dpi / 72.0

	faceBody, err := opentype.NewFace(regFont, &opentype.FaceOptions{
		Size:    13,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}
	defer faceBody.Close()

	faceHeader, err := opentype.NewFace(boldFont, &opentype.FaceOptions{
		Size:    13,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}
	defer faceHeader.Close()

	faceFooter, err := opentype.NewFace(regFont, &opentype.FaceOptions{
		Size:    11,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, err
	}
	defer faceFooter.Close()

	numCols := len(result.Columns)
	if numCols == 0 {
		return drawEmptyState(faceBody, fontScale), nil
	}

	// 1. Detect multi-level headers (e.g. "2024/Jan" or "WEST/Laptop")
	isMultiLevel := false
	topHeaders := make([]string, numCols)
	subHeaders := make([]string, numCols)

	for i, col := range result.Columns {
		if idx := strings.Index(col, "/"); idx != -1 {
			isMultiLevel = true
			topHeaders[i] = col[:idx]
			subHeaders[i] = col[idx+1:]
		} else {
			topHeaders[i] = col
			subHeaders[i] = ""
		}
	}

	// 2. Measure column widths based on cell and header contents
	colWidths := make([]int, numCols)
	paddingX := int(16 * fontScale) // 16px padding on each side
	rowHeight := int(32 * fontScale)
	headerRowHeight := int(34 * fontScale)
	footerHeight := int(28 * fontScale)
	margin := int(16 * fontScale)

	// Measure headers
	for i := 0; i < numCols; i++ {
		wTop := font.MeasureString(faceHeader, topHeaders[i]).Ceil()
		wSub := 0
		if isMultiLevel && subHeaders[i] != "" {
			wSub = font.MeasureString(faceHeader, subHeaders[i]).Ceil()
		}
		maxW := wTop
		if wSub > maxW {
			maxW = wSub
		}
		if maxW > colWidths[i] {
			colWidths[i] = maxW
		}
	}

	// Measure data rows
	isNumericCol := make([]bool, numCols)
	for i := range isNumericCol {
		isNumericCol[i] = true
	}

	for _, row := range result.Rows {
		for i := 0; i < numCols; i++ {
			val := ""
			if i < len(row) {
				val = formatCellValue(row[i])
			}
			if isNumericCol[i] && val != "" && val != "0" && !isNumeric(val) {
				isNumericCol[i] = false
			}
			w := font.MeasureString(faceBody, val).Ceil()
			if w > colWidths[i] {
				colWidths[i] = w
			}
		}
	}

	// Add padding to col widths & ensure minimum width
	minColWidth := int(70 * fontScale)
	for i := range colWidths {
		colWidths[i] += paddingX * 2
		if colWidths[i] < minColWidth {
			colWidths[i] = minColWidth
		}
	}

	// 3. Compute total dimensions
	tableWidth := 0
	for _, w := range colWidths {
		tableWidth += w
	}

	numHeaderRows := 1
	if isMultiLevel {
		numHeaderRows = 2
	}
	tableHeight := (numHeaderRows * headerRowHeight) + (len(result.Rows) * rowHeight) + footerHeight

	canvasWidth := tableWidth + (margin * 2)
	canvasHeight := tableHeight + (margin * 2)

	img := image.NewRGBA(image.Rect(0, 0, canvasWidth, canvasHeight))

	// Fill background
	fillRect(img, 0, 0, canvasWidth, canvasHeight, colorOuterBg)

	// Draw table container card with rounded-like fill
	tableX := margin
	tableY := margin
	fillRect(img, tableX, tableY, tableWidth, tableHeight, colorCardBg)

	// 4. Draw Header
	colXOffsets := make([]int, numCols+1)
	currX := tableX
	for i := 0; i < numCols; i++ {
		colXOffsets[i] = currX
		currX += colWidths[i]
	}
	colXOffsets[numCols] = currX

	if !isMultiLevel {
		// Single header row
		fillRect(img, tableX, tableY, tableWidth, headerRowHeight, colorHeaderBg)
		for i, col := range result.Columns {
			cx := colXOffsets[i]
			cw := colWidths[i]
			// Border separator
			if i > 0 {
				drawVLine(img, cx, tableY, headerRowHeight, colorHeaderBorder)
			}
			drawTextInCell(img, faceHeader, col, cx, tableY, cw, headerRowHeight, paddingX, isNumericCol[i], colorHeaderText)
		}
	} else {
		// Multi-level Header
		fillRect(img, tableX, tableY, tableWidth, headerRowHeight*2, colorHeaderBg)
		// Sub-header bar darker background
		fillRect(img, tableX, tableY+headerRowHeight, tableWidth, headerRowHeight, colorHeaderSubBg)

		// Draw Level 1 (Top headers with merged spans)
		i := 0
		for i < numCols {
			topName := topHeaders[i]
			startCol := i
			// Find span
			for i < numCols && topHeaders[i] == topName {
				i++
			}
			endCol := i - 1

			spanX := colXOffsets[startCol]
			spanW := colXOffsets[endCol+1] - spanX

			if subHeaders[startCol] == "" && startCol == endCol {
				// Vertical span (Rowspan 2)
				fillRect(img, spanX, tableY, spanW, headerRowHeight*2, colorHeaderBg)
				drawTextInCell(img, faceHeader, topName, spanX, tableY, spanW, headerRowHeight*2, paddingX, false, colorHeaderText)
			} else {
				// Horizontal span (Colspan)
				drawTextCentered(img, faceHeader, topName, spanX, tableY, spanW, headerRowHeight, colorHeaderText)
				// Horizontal border below top level
				drawHLine(img, spanX, tableY+headerRowHeight, spanW, colorHeaderBorder)

				// Draw sub headers
				for c := startCol; c <= endCol; c++ {
					subX := colXOffsets[c]
					subW := colWidths[c]
					if c > startCol {
						drawVLine(img, subX, tableY+headerRowHeight, headerRowHeight, colorHeaderBorder)
					}
					drawTextInCell(img, faceHeader, subHeaders[c], subX, tableY+headerRowHeight, subW, headerRowHeight, paddingX, isNumericCol[c], colorHeaderText)
				}
			}

			if startCol > 0 {
				drawVLine(img, spanX, tableY, headerRowHeight*2, colorHeaderBorder)
			}
		}
	}

	// 5. Draw Rows (Zebra stripes & cell text)
	dataStartY := tableY + (numHeaderRows * headerRowHeight)
	for r, row := range result.Rows {
		rowY := dataStartY + (r * rowHeight)
		rowBg := colorRowBgEven
		if r%2 == 1 {
			rowBg = colorRowBgOdd
		}
		fillRect(img, tableX, rowY, tableWidth, rowHeight, rowBg)

		// Draw horizontal cell border
		drawHLine(img, tableX, rowY, tableWidth, colorBorder)

		for c := 0; c < numCols; c++ {
			cx := colXOffsets[c]
			cw := colWidths[c]
			if c > 0 {
				drawVLine(img, cx, rowY, rowHeight, colorBorder)
			}
			val := ""
			if c < len(row) {
				val = formatCellValue(row[c])
			}
			drawTextInCell(img, faceBody, val, cx, rowY, cw, rowHeight, paddingX, isNumericCol[c], colorCellText)
		}
	}

	// 6. Draw Footer
	footerY := dataStartY + (len(result.Rows) * rowHeight)
	fillRect(img, tableX, footerY, tableWidth, footerHeight, colorFooterBg)
	drawHLine(img, tableX, footerY, tableWidth, colorBorder)

	footerText := fmt.Sprintf(" %d row(s) • %v", result.RowCount, result.Duration.Round(1_000_000))
	drawTextInCell(img, faceFooter, footerText, tableX, footerY, tableWidth, footerHeight, paddingX, false, colorFooterText)

	// Outer Table Border
	drawRectBorder(img, tableX, tableY, tableWidth, tableHeight, colorBorder)

	return img, nil
}

func formatCellValue(s string) string {
	if s == "" || s == "0" || s == "NULL" {
		return s
	}
	trimmed := s
	var neg bool
	if strings.HasPrefix(trimmed, "-") {
		neg = true
		trimmed = trimmed[1:]
	}
	parts := strings.Split(trimmed, ".")
	intPart := parts[0]
	for _, r := range intPart {
		if r < '0' || r > '9' {
			return s
		}
	}
	if len(intPart) <= 3 {
		return s
	}
	var sb strings.Builder
	if neg {
		sb.WriteByte('-')
	}
	n := len(intPart)
	for i, r := range intPart {
		if i > 0 && (n-i)%3 == 0 {
			sb.WriteByte(',')
		}
		sb.WriteRune(r)
	}
	if len(parts) > 1 {
		sb.WriteByte('.')
		sb.WriteString(parts[1])
	}
	return sb.String()
}

func isNumeric(s string) bool {
	if s == "" || s == "NULL" {
		return false
	}
	// Quick check
	hasDigit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			hasDigit = true
		} else if r != '.' && r != '-' && r != '+' && r != ',' && r != '%' {
			return false
		}
	}
	return hasDigit
}

func drawTextInCell(dst *image.RGBA, f font.Face, text string, x, y, w, h, padX int, alignRight bool, clr color.Color) {
	if text == "" {
		return
	}
	bounds, _ := font.BoundString(f, text)
	textW := (bounds.Max.X - bounds.Min.X).Ceil()
	fontHeight := f.Metrics().Ascent.Ceil()

	// Vertically center
	posY := y + (h+fontHeight)/2 - int(2*(f.Metrics().Descent.Ceil()/3))

	var posX int
	if alignRight {
		posX = x + w - padX - textW
	} else {
		posX = x + padX
	}

	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(clr),
		Face: f,
		Dot:  fixed.Point26_6{X: fixed.I(posX), Y: fixed.I(posY)},
	}
	d.DrawString(text)
}

func drawTextCentered(dst *image.RGBA, f font.Face, text string, x, y, w, h int, clr color.Color) {
	if text == "" {
		return
	}
	bounds, _ := font.BoundString(f, text)
	textW := (bounds.Max.X - bounds.Min.X).Ceil()
	fontHeight := f.Metrics().Ascent.Ceil()

	posY := y + (h+fontHeight)/2 - int(2*(f.Metrics().Descent.Ceil()/3))
	posX := x + (w-textW)/2

	d := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(clr),
		Face: f,
		Dot:  fixed.Point26_6{X: fixed.I(posX), Y: fixed.I(posY)},
	}
	d.DrawString(text)
}

func fillRect(dst *image.RGBA, x, y, w, h int, clr color.Color) {
	draw.Draw(dst, image.Rect(x, y, x+w, y+h), &image.Uniform{C: clr}, image.Point{}, draw.Src)
}

func drawHLine(dst *image.RGBA, x, y, w int, clr color.Color) {
	fillRect(dst, x, y, w, 1, clr)
}

func drawVLine(dst *image.RGBA, x, y, h int, clr color.Color) {
	fillRect(dst, x, y, 1, h, clr)
}

func drawRectBorder(dst *image.RGBA, x, y, w, h int, clr color.Color) {
	drawHLine(dst, x, y, w, clr)
	drawHLine(dst, x, y+h-1, w, clr)
	drawVLine(dst, x, y, h, clr)
	drawVLine(dst, x+w-1, y, h, clr)
}

func drawEmptyState(f font.Face, fontScale float64) *image.RGBA {
	w := int(300 * fontScale)
	h := int(100 * fontScale)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, 0, 0, w, h, colorOuterBg)
	drawTextCentered(img, f, "No data available", 0, 0, w, h, colorCellText)
	return img
}

// RenderPNGBytes returns the PNG bytes in memory.
func RenderPNGBytes(result *engine.Result) ([]byte, error) {
	var buf bytes.Buffer
	if err := RenderPNG(&buf, result); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
