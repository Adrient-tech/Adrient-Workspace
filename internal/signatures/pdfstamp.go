package signatures

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg" // signature images may be PNG or JPEG
	"image/png"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	pdfcrypto "github.com/Adrient-tech/Adrient-Workspace/internal/pdf/crypto"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// stampItem is one thing to draw on a page.
type stampItem struct {
	page       int
	text       string
	italic     bool
	imagePNG   []byte
	x, y, w, h float64 // fractions from the top-left of the page
}

// pdfEscapeText keeps only characters the core PDF fonts can draw.
func pdfText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 32 && r < 127:
			b.WriteRune(r)
		case r == 'é' || r == 'è' || r == 'ê':
			b.WriteRune('e')
		case r == 'ü' || r == 'ö' || r == 'ä':
			b.WriteRune(r - 0) // dropped below if unsupported
		default:
			b.WriteRune('?')
		}
	}
	return strings.NewReplacer("ü", "u", "ö", "o", "ä", "a").Replace(b.String())
}

// Stamp draws the items onto the original PDF.
func Stamp(orig []byte, items []stampItem) ([]byte, error) {
	conf := model.NewDefaultConfiguration()
	dims, err := api.PageDims(bytes.NewReader(orig), conf)
	if err != nil {
		return nil, fmt.Errorf("read the PDF: %w", err)
	}
	dir, err := os.MkdirTemp("", "adrient-sig-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	m := map[int][]*model.Watermark{}
	for i, it := range items {
		if it.page < 1 || it.page > len(dims) {
			continue
		}
		W, H := dims[it.page-1].Width, dims[it.page-1].Height
		bx, bw, bh := it.x*W, it.w*W, it.h*H
		by := H - (it.y+it.h)*H // bottom edge from the page bottom
		var wm *model.Watermark
		if it.imagePNG != nil {
			cfg, _, err := image.DecodeConfig(bytes.NewReader(it.imagePNG))
			if err != nil || cfg.Width == 0 || cfg.Height == 0 {
				continue
			}
			f := filepath.Join(dir, fmt.Sprintf("s%d.png", i))
			if err := os.WriteFile(f, it.imagePNG, 0o600); err != nil {
				return nil, err
			}
			// Fit inside the box, keeping the aspect ratio.
			scale := minf(bw/float64(cfg.Width), bh/float64(cfg.Height))
			iw, ih := float64(cfg.Width)*scale, float64(cfg.Height)*scale
			desc := fmt.Sprintf("pos:bl, off:%.2f %.2f, scale:%.5f abs, rot:0, op:1", bx+(bw-iw)/2, by+(bh-ih)/2, scale)
			wm, err = pdfcpu.ParseImageWatermarkDetails(f, desc, true, types.POINTS)
			if err != nil {
				return nil, err
			}
		} else {
			pts := minf(maxf(bh*0.6, 7), 16)
			if it.italic {
				pts = minf(maxf(bh*0.7, 9), 22)
			}
			font := "Helvetica"
			if it.italic {
				font = "Times-Italic"
			}
			desc := fmt.Sprintf("pos:bl, off:%.2f %.2f, scale:1 abs, rot:0, points:%d, fontname:%s, fillc:#0b1f5c, op:1", bx+2, by+(bh-pts)/2+1, int(pts), font)
			wm, err = pdfcpu.ParseTextWatermarkDetails(pdfText(it.text), desc, true, types.POINTS)
			if err != nil {
				return nil, err
			}
		}
		m[it.page] = append(m[it.page], wm)
	}
	if len(m) == 0 {
		return orig, nil
	}
	var out bytes.Buffer
	if err := api.AddWatermarksSliceMap(bytes.NewReader(orig), &out, m, conf); err != nil {
		return nil, fmt.Errorf("stamp the PDF: %w", err)
	}
	return out.Bytes(), nil
}

func minf(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// decodeSignatureImage turns a data URL (PNG or JPEG) into PNG bytes, refusing anything that is not an image.
func decodeSignatureImage(dataURL string) ([]byte, error) {
	i := strings.Index(dataURL, "base64,")
	if i < 0 || len(dataURL) > 400_000 {
		return nil, fmt.Errorf("the drawn signature is not valid or too large")
	}
	raw, err := base64.StdEncoding.DecodeString(dataURL[i+7:])
	if err != nil {
		return nil, fmt.Errorf("the drawn signature is not valid")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("the drawn signature is not an image")
	}
	b := img.Bounds()
	if b.Dx() < 20 || b.Dy() < 10 || b.Dx() > 2000 || b.Dy() > 1000 {
		return nil, fmt.Errorf("the drawn signature has an unusual size")
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ---- certificate page ----------------------------------------------------------

// certificatePDF draws a one-page completion certificate with the audit trail.
func certificatePDF(title, docHash string, signers []Signer, events []Event, headHash string) []byte {
	var lines []string
	add := func(s string) { lines = append(lines, pdfText(s)) }
	add("CERTIFICATE OF COMPLETION")
	add("")
	add("Document: " + title)
	add("Original SHA-256: " + docHash)
	add("")
	add("Signers")
	for _, s := range signers {
		if s.Role != "signer" {
			continue
		}
		when := ""
		if s.SignedAt != nil {
			when = s.SignedAt.UTC().Format("2006-01-02 15:04:05 UTC")
		}
		add(fmt.Sprintf("  %s <%s>  signed %s  IP %s", s.Name, s.Email, when, s.SignIP))
	}
	add("")
	add("Audit trail (each entry is chained to the one before it)")
	for _, e := range events {
		add(fmt.Sprintf("  %s  %-9s %s %s", e.At.UTC().Format("2006-01-02 15:04:05"), e.Event, trunc(e.Actor, 36), e.IP))
	}
	add("")
	add("Trail head: " + headHash)
	var content strings.Builder
	esc := strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`)
	y := 800
	for i, l := range lines {
		size := 9
		if i == 0 {
			size = 16
		}
		if len(l) > 105 {
			l = l[:105]
		}
		fmt.Fprintf(&content, "BT /F1 %d Tf 40 %d Td (%s) Tj ET\n", size, y, esc.Replace(l))
		y -= size + 6
		if y < 40 {
			break
		}
	}
	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Courier >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", content.Len(), content.String()),
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	var offs []int
	for i, o := range objs {
		offs = append(offs, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, x)
	return b.Bytes()
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// appendPage adds the certificate after the last page of pdf.
func appendPage(pdf, cert []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := api.MergeRaw([]io.ReadSeeker{bytes.NewReader(pdf), bytes.NewReader(cert)}, &out, false, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ---- seal ------------------------------------------------------------------------

// sealCA is a one-off self-signed authority: the seal proves the completed file was not changed afterwards.
type sealCA struct {
	cert *x509.Certificate
	key  crypto.PrivateKey
}

func (s sealCA) GetOrCreateSignerCertificate(ctx context.Context, orgID, email, name string) (*x509.Certificate, crypto.PrivateKey, error) {
	return s.cert, s.key, nil
}
func (s sealCA) GetCACertificate() *x509.Certificate { return s.cert }

func newSealCA(org string, now time.Time) (sealCA, error) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return sealCA{}, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
	tpl := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "Adrient e-signature seal", Organization: []string{org}},
		NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(10, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	if err != nil {
		return sealCA{}, err
	}
	c, err := x509.ParseCertificate(der)
	return sealCA{cert: c, key: k}, err
}

// Seal adds a cryptographic document signature (self-signed, so it proves integrity, not identity).
func Seal(pdf []byte, org, reason string, now time.Time) ([]byte, error) {
	ca, err := newSealCA(org, now)
	if err != nil {
		return nil, err
	}
	out, _, err := pdfcrypto.NewPDFSigner(ca).SignPDF(pdf, ca.cert, ca.key.(crypto.Signer), pdfcrypto.SignOptions{Name: "Adrient e-signature seal", Reason: reason})
	return out, err
}
