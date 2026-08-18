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
	// Custom Telkomsel / Dashboard colors
	colorHeaderDarkGray = color.RGBA{R: 89, G: 89, B: 89, A: 255}
	colorHeaderBlue     = color.RGBA{R: 11, G: 58, B: 130, A: 255}
	colorHeaderRed      = color.RGBA{R: 225, G: 25, B: 49, A: 255}
	colorHeaderBlack    = color.RGBA{R: 0, G: 0, B: 0, A: 255}

	colorHeaderText   = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	colorRowBgEven    = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	colorRowBgOdd     = color.RGBA{R: 243, G: 244, B: 246, A: 255} // Light gray odd row
	colorRowRegion    = color.RGBA{R: 200, G: 200, B: 200, A: 255} // Light/Mid gray for regions
	colorRowTotal     = color.RGBA{R: 0, G: 0, B: 0, A: 255}       // Black
	colorRowBlue      = color.RGBA{R: 11, G: 58, B: 130, A: 255}   // Blue for %Cont
	colorCellTotalCol = color.RGBA{R: 100, G: 100, B: 100, A: 255} // Subtotal col background

	colorBorder       = color.RGBA{R: 229, G: 231, B: 235, A: 255}
	colorHeaderBorder = color.RGBA{R: 255, G: 255, B: 255, A: 140}
	colorCellText     = color.RGBA{R: 31, G: 41, B: 55, A: 255}
	colorFooterBg     = color.RGBA{R: 248, G: 249, B: 251, A: 255}
	colorFooterText   = color.RGBA{R: 107, G: 114, B: 128, A: 255}
	colorCardBg       = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	colorOuterBg      = color.RGBA{R: 243, G: 244, B: 246, A: 255}
)

func RenderPNG(w io.Writer, result *engine.Result) error {
	img, err := DrawTableImage(result)
	if err != nil {
		return err
	}
	return png.Encode(w, img)
}

func getHeaderColor(text string) color.Color {
	t := strings.ToLower(text)
	if strings.Contains(t, "%progress") || strings.Contains(t, "%cont") {
		return colorHeaderBlue
	}
	if strings.HasPrefix(t, "total ") {
		return colorHeaderRed
	}
	if t == "grand total" {
		return colorHeaderBlack
	}
	return colorHeaderDarkGray
}

func DrawTableImage(result *engine.Result) (*image.RGBA, error) {
	regFont, _ := opentype.Parse(goregular.TTF)
	boldFont, _ := opentype.Parse(gobold.TTF)

	const dpi = 144.0
	fontScale := dpi / 72.0

	faceBody, _ := opentype.NewFace(regFont, &opentype.FaceOptions{Size: 13, DPI: dpi, Hinting: font.HintingFull})
	faceBodyBold, _ := opentype.NewFace(boldFont, &opentype.FaceOptions{Size: 13, DPI: dpi, Hinting: font.HintingFull})
	faceHeader, _ := opentype.NewFace(boldFont, &opentype.FaceOptions{Size: 13, DPI: dpi, Hinting: font.HintingFull})
	faceFooter, _ := opentype.NewFace(regFont, &opentype.FaceOptions{Size: 11, DPI: dpi, Hinting: font.HintingFull})

	numCols := len(result.Columns)
	if numCols == 0 {
		return drawEmptyState(faceBody, fontScale), nil
	}

	// Detect up to 3 levels of headers
	numHeaderRows := 1
	topHeaders := make([]string, numCols)
	midHeaders := make([]string, numCols)
	subHeaders := make([]string, numCols)

	for i, col := range result.Columns {
		parts := strings.Split(col, "/")
		if len(parts) == 3 {
			if numHeaderRows < 3 {
				numHeaderRows = 3
			}
			topHeaders[i] = parts[0]
			midHeaders[i] = parts[1]
			subHeaders[i] = parts[2]
		} else if len(parts) == 2 {
			if numHeaderRows < 2 {
				numHeaderRows = 2
			}
			topHeaders[i] = parts[0]
			midHeaders[i] = parts[1]
			subHeaders[i] = ""
		} else {
			topHeaders[i] = col
			midHeaders[i] = ""
			subHeaders[i] = ""
		}
	}

	colWidths := make([]int, numCols)
	paddingX := int(16 * fontScale)
	rowHeight := int(32 * fontScale)
	headerRowHeight := int(34 * fontScale)
	footerHeight := int(28 * fontScale)
	margin := int(16 * fontScale)

	// Measure headers
	for i := 0; i < numCols; i++ {
		wTop := font.MeasureString(faceHeader, topHeaders[i]).Ceil()
		wMid := font.MeasureString(faceHeader, midHeaders[i]).Ceil()
		wSub := font.MeasureString(faceHeader, subHeaders[i]).Ceil()
		maxW := wTop
		if wMid > maxW { maxW = wMid }
		if wSub > maxW { maxW = wSub }
		if maxW > colWidths[i] {
			colWidths[i] = maxW
		}
	}

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
			w := font.MeasureString(faceBodyBold, val).Ceil() // measure with bold to be safe
			if w > colWidths[i] {
				colWidths[i] = w
			}
		}
	}

	minColWidth := int(70 * fontScale)
	for i := range colWidths {
		colWidths[i] += paddingX * 2
		if colWidths[i] < minColWidth {
			colWidths[i] = minColWidth
		}
	}

	tableWidth := 0
	for _, w := range colWidths {
		tableWidth += w
	}
	tableHeight := (numHeaderRows * headerRowHeight) + (len(result.Rows) * rowHeight) + footerHeight

	canvasWidth := tableWidth + (margin * 2)
	canvasHeight := tableHeight + (margin * 2)

	img := image.NewRGBA(image.Rect(0, 0, canvasWidth, canvasHeight))
	fillRect(img, 0, 0, canvasWidth, canvasHeight, colorOuterBg)

	tableX := margin
	tableY := margin
	fillRect(img, tableX, tableY, tableWidth, tableHeight, colorCardBg)

	colXOffsets := make([]int, numCols+1)
	currX := tableX
	for i := 0; i < numCols; i++ {
		colXOffsets[i] = currX
		currX += colWidths[i]
	}
	colXOffsets[numCols] = currX

	// Draw Headers
	for i := 0; i < numCols; {
		topName := topHeaders[i]
		startCol := i
		for i < numCols && topHeaders[i] == topName {
			i++
		}
		endCol := i - 1
		spanX := colXOffsets[startCol]
		spanW := colXOffsets[endCol+1] - spanX
		
		bgClr := getHeaderColor(topName)
		
		if midHeaders[startCol] == "" && subHeaders[startCol] == "" && startCol == endCol {
			// Rowspan full height
			fillRect(img, spanX, tableY, spanW, headerRowHeight*numHeaderRows, bgClr)
			drawTextInCell(img, faceHeader, topName, spanX, tableY, spanW, headerRowHeight*numHeaderRows, paddingX, false, colorHeaderText)
		} else {
			// Top level
			fillRect(img, spanX, tableY, spanW, headerRowHeight, bgClr)
			drawTextCentered(img, faceHeader, topName, spanX, tableY, spanW, headerRowHeight, colorHeaderText)
			
			// Mid and Sub levels
			for j := startCol; j <= endCol; {
				midName := midHeaders[j]
				midStart := j
				for j <= endCol && midHeaders[j] == midName {
					j++
				}
				midEnd := j - 1
				midSpanX := colXOffsets[midStart]
				midSpanW := colXOffsets[midEnd+1] - midSpanX
				
				midBgClr := bgClr
				if strings.Contains(strings.ToLower(midName), "total") {
					midBgClr = colorCellTotalCol
				}
				
				if subHeaders[midStart] == "" && midStart == midEnd {
					// Mid spans remaining height
					remH := headerRowHeight * (numHeaderRows - 1)
					fillRect(img, midSpanX, tableY+headerRowHeight, midSpanW, remH, midBgClr)
					drawHLine(img, midSpanX, tableY+headerRowHeight, midSpanW, colorHeaderBorder)
					drawTextInCell(img, faceHeader, midName, midSpanX, tableY+headerRowHeight, midSpanW, remH, paddingX, isNumericCol[midStart], colorHeaderText)
				} else {
					// Mid level
					fillRect(img, midSpanX, tableY+headerRowHeight, midSpanW, headerRowHeight, midBgClr)
					drawHLine(img, midSpanX, tableY+headerRowHeight, midSpanW, colorHeaderBorder)
					drawTextCentered(img, faceHeader, midName, midSpanX, tableY+headerRowHeight, midSpanW, headerRowHeight, colorHeaderText)
					
					// Sub level
					if numHeaderRows == 3 {
						for k := midStart; k <= midEnd; k++ {
							subX := colXOffsets[k]
							subW := colWidths[k]
							
							subBgClr := bgClr
							if strings.Contains(strings.ToLower(subHeaders[k]), "total") || strings.Contains(strings.ToLower(midName), "total") {
								subBgClr = colorCellTotalCol
							}
							
							fillRect(img, subX, tableY+headerRowHeight*2, subW, headerRowHeight, subBgClr)
							drawHLine(img, subX, tableY+headerRowHeight*2, subW, colorHeaderBorder)
							if k > midStart {
								drawVLine(img, subX, tableY+headerRowHeight*2, headerRowHeight, colorHeaderBorder)
							}
							drawTextInCell(img, faceHeader, subHeaders[k], subX, tableY+headerRowHeight*2, subW, headerRowHeight, paddingX, isNumericCol[k], colorHeaderText)
						}
					}
				}
				if midStart > startCol {
					drawVLine(img, midSpanX, tableY+headerRowHeight, headerRowHeight*(numHeaderRows-1), colorHeaderBorder)
				}
			}
		}
		if startCol > 0 {
			drawVLine(img, spanX, tableY, headerRowHeight*numHeaderRows, colorHeaderBorder)
		}
	}

	// 5. Draw Rows
	dataStartY := tableY + (numHeaderRows * headerRowHeight)
	for r, row := range result.Rows {
		rowY := dataStartY + (r * rowHeight)
		
		isGrandTotal := len(row) > 0 && row[0] == "Grand Total"
		isContByMonth := len(row) > 0 && strings.Contains(row[0], "%Cont")
		isRegion := len(row) > 0 && !strings.HasPrefix(row[0], " ") && !isGrandTotal && !isContByMonth
		
		var rowBg color.Color = colorRowBgEven
		if r%2 == 1 { rowBg = colorRowBgOdd }
		
		textColor := colorCellText
		currFace := faceBody
		
		if isGrandTotal {
			rowBg = colorRowTotal
			textColor = colorHeaderText
			currFace = faceBodyBold
		} else if isContByMonth {
			rowBg = colorRowBlue
			textColor = colorHeaderText
			currFace = faceBodyBold
		} else if isRegion {
			rowBg = colorRowRegion
			textColor = colorCellText
			currFace = faceBodyBold
		}

		fillRect(img, tableX, rowY, tableWidth, rowHeight, rowBg)
		drawHLine(img, tableX, rowY, tableWidth, colorBorder)

		for c := 0; c < numCols; c++ {
			cx := colXOffsets[c]
			cw := colWidths[c]
			
			// Subtotal columns in dark gray
			if !isGrandTotal && !isContByMonth && !isRegion {
				if strings.Contains(strings.ToLower(midHeaders[c]), "total") || strings.Contains(strings.ToLower(subHeaders[c]), "total") {
					fillRect(img, cx, rowY, cw, rowHeight, colorCellTotalCol)
					if textColor == colorCellText {
						// draw text white inside total cols
						drawTextInCell(img, currFace, formatCellValue(row[c]), cx, rowY, cw, rowHeight, paddingX, isNumericCol[c], colorHeaderText)
						if c > 0 { drawVLine(img, cx, rowY, rowHeight, colorBorder) }
						continue
					}
				}
			}

			if c > 0 {
				drawVLine(img, cx, rowY, rowHeight, colorBorder)
			}
			val := ""
			if c < len(row) {
				val = formatCellValue(row[c])
				if c == 0 && !isRegion && !isGrandTotal && !isContByMonth {
					val = strings.TrimSpace(val) // remove space prefix for display
				}
			}
			drawTextInCell(img, currFace, val, cx, rowY, cw, rowHeight, paddingX, isNumericCol[c], textColor)
		}
	}

	// 6. Draw Footer
	footerY := dataStartY + (len(result.Rows) * rowHeight)
	fillRect(img, tableX, footerY, tableWidth, footerHeight, colorFooterBg)
	drawHLine(img, tableX, footerY, tableWidth, colorBorder)

	footerText := fmt.Sprintf(" %d row(s) • %v", result.RowCount, result.Duration.Round(1_000_000))
	drawTextInCell(img, faceFooter, footerText, tableX, footerY, tableWidth, footerHeight, paddingX, false, colorFooterText)

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
	if s == "" || s == "NULL" { return false }
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
	if text == "" { return }
	bounds, _ := font.BoundString(f, text)
	textW := (bounds.Max.X - bounds.Min.X).Ceil()
	fontHeight := f.Metrics().Ascent.Ceil()

	posY := y + (h+fontHeight)/2 - int(2*(f.Metrics().Descent.Ceil()/3))
	posX := x + padX
	if alignRight { posX = x + w - padX - textW }

	d := &font.Drawer{Dst: dst, Src: image.NewUniform(clr), Face: f, Dot: fixed.Point26_6{X: fixed.I(posX), Y: fixed.I(posY)}}
	d.DrawString(text)
}

func drawTextCentered(dst *image.RGBA, f font.Face, text string, x, y, w, h int, clr color.Color) {
	if text == "" { return }
	bounds, _ := font.BoundString(f, text)
	textW := (bounds.Max.X - bounds.Min.X).Ceil()
	fontHeight := f.Metrics().Ascent.Ceil()

	posY := y + (h+fontHeight)/2 - int(2*(f.Metrics().Descent.Ceil()/3))
	posX := x + (w-textW)/2

	d := &font.Drawer{Dst: dst, Src: image.NewUniform(clr), Face: f, Dot: fixed.Point26_6{X: fixed.I(posX), Y: fixed.I(posY)}}
	d.DrawString(text)
}

func fillRect(dst *image.RGBA, x, y, w, h int, clr color.Color) {
	draw.Draw(dst, image.Rect(x, y, x+w, y+h), &image.Uniform{C: clr}, image.Point{}, draw.Src)
}

func drawHLine(dst *image.RGBA, x, y, w int, clr color.Color) { fillRect(dst, x, y, w, 1, clr) }
func drawVLine(dst *image.RGBA, x, y, h int, clr color.Color) { fillRect(dst, x, y, 1, h, clr) }
func drawRectBorder(dst *image.RGBA, x, y, w, h int, clr color.Color) {
	drawHLine(dst, x, y, w, clr); drawHLine(dst, x, y+h-1, w, clr)
	drawVLine(dst, x, y, h, clr); drawVLine(dst, x+w-1, y, h, clr)
}

func drawEmptyState(f font.Face, fontScale float64) *image.RGBA {
	w, h := int(300*fontScale), int(100*fontScale)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, 0, 0, w, h, colorOuterBg)
	drawTextCentered(img, f, "No data available", 0, 0, w, h, colorCellText)
	return img
}

func RenderPNGBytes(result *engine.Result) ([]byte, error) {
	var buf bytes.Buffer
	if err := RenderPNG(&buf, result); err != nil { return nil, err }
	return buf.Bytes(), nil
}
