package pdf

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// PageFont describes one font used on a page. File holds the embedded
// TrueType or OpenType program when the file carries one (bare CFF and Type 1
// programs are not returned: they cannot be re-embedded as they are).
type PageFont struct {
	Resource string `json:"resource"`
	BaseFont string `json:"baseFont"`
	Subtype  string `json:"subtype"`
	Bold     bool   `json:"bold"`
	Italic   bool   `json:"italic"`
	Format   string `json:"format,omitempty"` // "truetype" | "opentype"
	File     []byte `json:"file,omitempty"`
}

// maxFontFile bounds one returned font program.
const maxFontFile = 8 << 20

// PageFonts lists the fonts of a page (1-based) with their embedded programs.
func PageFonts(in []byte, pageNr int) ([]PageFont, error) {
	ctx, err := api.ReadContext(bytes.NewReader(in), model.NewDefaultConfiguration())
	if err != nil {
		return nil, fmt.Errorf("read pdf: %w", err)
	}
	if err := ctx.EnsurePageCount(); err != nil {
		return nil, err
	}
	if pageNr < 1 || pageNr > ctx.PageCount {
		return nil, fmt.Errorf("page %d out of range 1..%d", pageNr, ctx.PageCount)
	}
	d, _, inh, err := ctx.PageDict(pageNr, false)
	if err != nil || d == nil {
		return nil, fmt.Errorf("page %d: %v", pageNr, err)
	}
	var res types.Dict
	if o, found := d.Find("Resources"); found {
		res, _ = ctx.DereferenceDict(o)
	} else if inh != nil {
		res = inh.Resources
	}
	if res == nil {
		return nil, nil
	}
	fonts, _ := ctx.DereferenceDict(res["Font"])
	out := []PageFont{}
	for name, o := range fonts {
		fd, err := ctx.DereferenceDict(o)
		if err != nil || fd == nil {
			continue
		}
		pf := PageFont{Resource: name}
		if v := fd.NameEntry("BaseFont"); v != nil {
			pf.BaseFont = *v
		}
		if v := fd.NameEntry("Subtype"); v != nil {
			pf.Subtype = *v
		}
		desc := fd
		if pf.Subtype == "Type0" {
			if da, _ := ctx.DereferenceArray(fd["DescendantFonts"]); len(da) > 0 {
				if cd, _ := ctx.DereferenceDict(da[0]); cd != nil {
					desc = cd
				}
			}
		}
		fdesc, _ := ctx.DereferenceDict(desc["FontDescriptor"])
		lower := strings.ToLower(pf.BaseFont)
		pf.Bold = strings.Contains(lower, "bold") || strings.Contains(lower, "black") || strings.Contains(lower, "heavy")
		pf.Italic = strings.Contains(lower, "italic") || strings.Contains(lower, "oblique")
		if fdesc != nil {
			if fl, ok := numOf(ctx, fdesc["Flags"]); ok {
				f := int(fl)
				pf.Italic = pf.Italic || f&(1<<6) != 0
				pf.Bold = pf.Bold || f&(1<<18) != 0
			}
			if w, ok := numOf(ctx, fdesc["FontWeight"]); ok && w >= 600 {
				pf.Bold = true
			}
			pf.Format, pf.File = fontProgram(ctx, fdesc)
		}
		out = append(out, pf)
	}
	return out, nil
}

func fontProgram(ctx *model.Context, fdesc types.Dict) (string, []byte) {
	load := func(key string) []byte {
		sd, _, err := ctx.DereferenceStreamDict(fdesc[key])
		if err != nil || sd == nil {
			return nil
		}
		if err := sd.DecodeWithLimit(maxFontFile); err != nil {
			return nil
		}
		if len(sd.Content) > maxFontFile {
			return nil
		}
		return sd.Content
	}
	if b := load("FontFile2"); b != nil {
		return "truetype", b
	}
	sd, _, err := ctx.DereferenceStreamDict(fdesc["FontFile3"])
	if err == nil && sd != nil {
		if st := sd.Dict.NameEntry("Subtype"); st != nil && *st == "OpenType" {
			if b := load("FontFile3"); b != nil {
				return "opentype", b
			}
		}
	}
	return "", nil
}
