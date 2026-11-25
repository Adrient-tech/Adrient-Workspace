package pdf

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"

	"github.com/pdfcpu/pdfcpu/pkg/filter"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// maxClipPixels bounds the images we decode for pixel redaction.
const maxClipPixels = 40_000_000

// clipImage returns a copy of an image XObject with the pixels under the
// redaction regions painted black, so the covered part of the picture is gone
// from the file while the rest stays. ctm maps the image's unit square to
// page space. Supported: JPEG (DCTDecode) and 8-bit gray/RGB/CMYK samples,
// unfiltered or Flate-compressed. ok is false for anything else.
func clipImage(sd *types.StreamDict, ctm matrix, regions []rect) (*types.StreamDict, bool) {
	inv, ok := ctm.invert()
	if !ok {
		return nil, false
	}
	w := sd.Dict.IntEntry("Width")
	h := sd.Dict.IntEntry("Height")
	if w == nil || h == nil || *w <= 0 || *h <= 0 || int64(*w)*int64(*h) > maxClipPixels {
		return nil, false
	}
	W, H := *w, *h
	// Pixel rectangles (row 0 at the top of the image) covering each region.
	var boxes []image.Rectangle
	for _, r := range regions {
		u := rect{1e18, 1e18, -1e18, -1e18}
		for _, p := range [][2]float64{{r.x0, r.y0}, {r.x1, r.y0}, {r.x0, r.y1}, {r.x1, r.y1}} {
			x, y := inv.apply(p[0], p[1])
			u.x0, u.y0 = min(u.x0, x), min(u.y0, y)
			u.x1, u.y1 = max(u.x1, x), max(u.y1, y)
		}
		b := image.Rect(
			int(math.Floor(u.x0*float64(W))), int(math.Floor((1-u.y1)*float64(H))),
			int(math.Ceil(u.x1*float64(W))), int(math.Ceil((1-u.y0)*float64(H))),
		).Intersect(image.Rect(0, 0, W, H))
		if !b.Empty() {
			boxes = append(boxes, b)
		}
	}
	if len(boxes) == 0 {
		return nil, false
	}
	if mask := sd.Dict.BooleanEntry("ImageMask"); mask != nil && *mask {
		return nil, false
	}
	filters := sd.FilterPipeline
	switch {
	case len(filters) == 1 && filters[0].Name == filter.DCT:
		img, err := jpeg.Decode(bytes.NewReader(sd.Raw))
		if err != nil || img.Bounds().Dx() != W || img.Bounds().Dy() != H {
			return nil, false
		}
		var dst draw.Image
		switch img.(type) {
		case *image.Gray:
			dst = image.NewGray(img.Bounds())
		case *image.CMYK:
			// CMYK JPEGs differ in their inversion convention; re-encoding
			// cannot keep it reliably, so the caller drops the image whole.
			return nil, false
		default:
			dst = image.NewRGBA(img.Bounds())
		}
		draw.Draw(dst, dst.Bounds(), img, image.Point{}, draw.Src)
		for _, b := range boxes {
			draw.Draw(dst, b, &image.Uniform{color.Black}, image.Point{}, draw.Src)
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 90}); err != nil {
			return nil, false
		}
		nd := copyImageDict(sd.Dict)
		nsd := types.StreamDict{Dict: nd, Content: buf.Bytes(), Raw: buf.Bytes(),
			FilterPipeline: []types.PDFFilter{{Name: filter.DCT}}}
		nsd.InsertName("Filter", filter.DCT)
		l := int64(buf.Len())
		nsd.StreamLength = &l
		nsd.Dict.Update("Length", types.Integer(l))
		return &nsd, true
	case len(filters) == 0 || (len(filters) == 1 && filters[0].Name == filter.Flate):
		bpc := sd.Dict.IntEntry("BitsPerComponent")
		if bpc == nil || *bpc != 8 {
			return nil, false
		}
		if err := sd.Decode(); err != nil {
			return nil, false
		}
		ncomp := len(sd.Content) / (W * H)
		if ncomp < 1 || ncomp > 4 || len(sd.Content) != ncomp*W*H {
			return nil, false
		}
		px := append([]byte(nil), sd.Content...)
		// Black is 0 in gray and RGB samples, and K=255 with C=M=Y=0 in CMYK.
		for _, b := range boxes {
			for y := b.Min.Y; y < b.Max.Y; y++ {
				row := px[(y*W+b.Min.X)*ncomp : (y*W+b.Max.X)*ncomp]
				for i := range row {
					row[i] = 0
					if ncomp == 4 && i%4 == 3 {
						row[i] = 255
					}
				}
			}
		}
		nd := copyImageDict(sd.Dict)
		nd.Delete("DecodeParms")
		nsd := types.StreamDict{Dict: nd, Content: px,
			FilterPipeline: []types.PDFFilter{{Name: filter.Flate}}}
		nsd.InsertName("Filter", filter.Flate)
		if err := nsd.Encode(); err != nil {
			return nil, false
		}
		return &nsd, true
	}
	return nil, false
}

func copyImageDict(d types.Dict) types.Dict {
	nd := types.NewDict()
	for k, v := range d {
		switch k {
		case "Filter", "DecodeParms", "Length":
			continue
		}
		nd[k] = v
	}
	return nd
}

// invert returns the inverse of an affine matrix.
func (m matrix) invert() (matrix, bool) {
	det := m[0]*m[3] - m[1]*m[2]
	if math.Abs(det) < 1e-12 {
		return matrix{}, false
	}
	a, b, c, d := m[3]/det, -m[1]/det, -m[2]/det, m[0]/det
	return matrix{a, b, c, d, -(m[4]*a + m[5]*c), -(m[4]*b + m[5]*d)}, true
}
