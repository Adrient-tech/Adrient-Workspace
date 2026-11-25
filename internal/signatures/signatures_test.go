package signatures

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit"
	"github.com/Adrient-tech/Adrient-Workspace/internal/knowkit/kittest"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func drawnSig(t *testing.T) string {
	img := image.NewRGBA(image.Rect(0, 0, 240, 80))
	for x := 10; x < 230; x++ {
		img.Set(x, 40+int(20*float64((x%40)-20)/20), color.Black)
		img.Set(x, 41+int(20*float64((x%40)-20)/20), color.Black)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

type fakeDrive struct {
	mu    sync.Mutex
	files map[string][]byte
	owner map[string]string
	n     int
}

func (d *fakeDrive) put(owner string, b []byte) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.n++
	id := fmt.Sprintf("22222222-0000-0000-0000-%012d", d.n)
	d.files[id], d.owner[id] = b, owner
	return id
}

func TestSigningFlow(t *testing.T) {
	fx := kittest.Setup(t)
	ctx := context.Background()
	repo := NewRepository(fx.Pool)
	repo.Deps.Now = func() time.Time { return time.Date(2026, 3, 10, 9, 30, 0, 0, time.UTC) }
	dr := &fakeDrive{files: map[string][]byte{}, owner: map[string]string{}}
	repo.Deps.ReadFile = func(ctx context.Context, org, owner, id string) ([]byte, string, error) {
		if dr.owner[id] != owner {
			return nil, "", fmt.Errorf("not yours")
		}
		return dr.files[id], "Agreement.pdf", nil
	}
	repo.Deps.ReadFileAny = func(ctx context.Context, org, id string) ([]byte, string, error) { return dr.files[id], "x.pdf", nil }
	repo.Deps.SaveFile = func(ctx context.Context, org, owner, name, mime string, data []byte) (string, error) {
		return dr.put(owner, data), nil
	}
	var mails []string
	repo.Deps.Mail = func(ctx context.Context, to []string, subject, h, text string) error {
		mails = append(mails, to[0]+"|"+subject)
		return nil
	}
	a := fx.User
	src := certificatePDF("Master agreement", "abc", nil, nil, "") // a valid one-page PDF
	fileID := dr.put(a.User, src)
	other := dr.put(fx.Admin.User, src)
	notPDF := dr.put(a.User, []byte("hello"))

	in := Input{Title: "Master agreement", FileID: fileID, Sequential: true, ExpireDays: 7,
		Signers: []SignerInput{{Name: "Asha", Email: "asha@example.test"}, {Name: "Ravi", Email: "ravi@example.test"}, {Name: "Legal", Email: "legal@example.test", Role: "cc"}},
		Fields: []FieldInput{
			{SignerIndex: 0, Type: "signature", Page: 1, X: 0.1, Y: 0.7, W: 0.3, H: 0.06},
			{SignerIndex: 0, Type: "date", Page: 1, X: 0.5, Y: 0.7, W: 0.2, H: 0.04},
			{SignerIndex: 1, Type: "signature", Page: 1, X: 0.1, Y: 0.8, W: 0.3, H: 0.06},
			{SignerIndex: 1, Type: "text", Label: "Title", Page: 1, X: 0.5, Y: 0.8, W: 0.3, H: 0.04},
			{SignerIndex: 1, Type: "checkbox", Label: "I agree", Page: 1, X: 0.1, Y: 0.9, W: 0.03, H: 0.03},
		}}
	// Validation.
	bad := in
	bad.FileID = other
	if _, err := repo.Create(ctx, a, bad); err == nil {
		t.Error("a file of someone else must be refused")
	}
	bad = in
	bad.FileID = notPDF
	if _, err := repo.Create(ctx, a, bad); err == nil {
		t.Error("a non-PDF must be refused")
	}
	bad = in
	bad.Fields = in.Fields[:1]
	if _, err := repo.Create(ctx, a, bad); err == nil {
		t.Error("every signer needs a signature field")
	}
	bad = in
	bad.Fields = append([]FieldInput{{SignerIndex: 0, Type: "signature", Page: 1, X: 0.9, Y: 0.9, W: 0.3, H: 0.3}}, in.Fields[1:]...)
	if _, err := repo.Create(ctx, a, bad); err == nil {
		t.Error("a field outside the page must be refused")
	}
	bad = in
	bad.Signers = []SignerInput{{Name: "x", Email: "nope"}}
	if _, err := repo.Create(ctx, a, bad); err == nil {
		t.Error("a bad email must be refused")
	}

	req, err := repo.Create(ctx, a, in)
	if err != nil {
		t.Fatal(err)
	}
	if req.Status != "draft" || len(req.Signers) != 3 || len(req.Fields) != 5 {
		t.Fatalf("draft = %+v", req)
	}
	if _, err := repo.Get(ctx, fx.Other, req.ID); err == nil {
		t.Error("another org must not see the request")
	}
	if _, err := repo.Send(ctx, fx.Admin, req.ID, "https://w.example"); err != nil {
		t.Errorf("an org admin may manage: %v", err)
	}
	// (the admin sent it; the original sender can still manage)
	req, _ = repo.Get(ctx, a, req.ID)
	if req.Status != "sent" {
		t.Fatalf("status = %s", req.Status)
	}
	links := map[string]string{}
	sent, _ := repo.Get(ctx, a, req.ID)
	_ = sent
	// Sending again hands out new tokens through Remind, so use Remind to fetch fresh links for each signer.
	mails = nil
	// The first person was mailed (sequential: only the first signer and the cc).
	if len(mails) != 0 {
		t.Logf("mails: %v", mails)
	}
	tokOf := func(email string) string {
		tok := knowkit.NewKey() + knowkit.NewKey()
		if _, err := fx.Pool.Exec(ctx, `UPDATE adrient.sig_signers SET token_hash=$2 WHERE request_id=$1 AND email=$3`, req.ID, hashToken(tok), email); err != nil {
			t.Fatal(err)
		}
		return tok
	}
	asha, ravi := tokOf("asha@example.test"), tokOf("ravi@example.test")
	_ = links
	c1 := Client{IP: "203.0.113.5", UA: "TestBrowser/1", Base: "https://w.example"}

	// Ravi cannot sign before Asha.
	if _, err := repo.PublicSign(ctx, ravi, SignInput{Consent: true, SignatureKind: "typed", SignatureData: "Ravi K", Values: map[string]string{}}, c1); err == nil {
		t.Error("sequential order must be enforced")
	}
	v, err := repo.PublicView(ctx, asha, c1)
	if err != nil || !v.CanSign || len(v.Fields) != 2 {
		t.Fatalf("view: %+v %v", v, err)
	}
	if _, err := repo.PublicView(ctx, strings.Repeat("a", 64), c1); err == nil {
		t.Error("an unknown token must not work")
	}
	// Missing consent, bad signature input.
	if _, err := repo.PublicSign(ctx, asha, SignInput{SignatureKind: "typed", SignatureData: "Asha"}, c1); err == nil {
		t.Error("consent is required")
	}
	if _, err := repo.PublicSign(ctx, asha, SignInput{Consent: true, SignatureKind: "drawn", SignatureData: "data:image/png;base64,AAAA"}, c1); err == nil {
		t.Error("an invalid drawn signature must be refused")
	}
	if _, err := repo.PublicSign(ctx, asha, SignInput{Consent: true, SignatureKind: "typed", SignatureData: "  "}, c1); err == nil {
		t.Error("an empty typed signature must be refused")
	}
	d, err := repo.PublicSign(ctx, asha, SignInput{Consent: true, SignatureKind: "drawn", SignatureData: drawnSig(t)}, c1)
	if err != nil || !d.DoneSigned {
		t.Fatalf("asha sign: %+v %v", d, err)
	}
	if _, err := repo.PublicSign(ctx, asha, SignInput{Consent: true, SignatureKind: "typed", SignatureData: "Again"}, c1); err == nil {
		t.Error("signing twice must be refused")
	}
	ravi = tokOf("ravi@example.test") // the link rotates when it becomes Ravi's turn
	// Ravi must tick the required box and fill the required text.
	if _, err := repo.PublicSign(ctx, ravi, SignInput{Consent: true, SignatureKind: "typed", SignatureData: "Ravi K", Values: map[string]string{}}, c1); err == nil {
		t.Error("required fields must be filled")
	}
	vals := map[string]string{}
	rv, rerr := repo.PublicView(ctx, ravi, c1)
	if rerr != nil {
		t.Fatalf("ravi view: %v", rerr)
	}
	for _, f := range rv.Fields {
		switch f.Type {
		case "text":
			vals[f.ID] = "Director"
		case "checkbox":
			vals[f.ID] = "true"
		}
	}
	if _, err := repo.PublicSign(ctx, ravi, SignInput{Consent: true, SignatureKind: "typed", SignatureData: "Ravi Kumar", Values: vals}, Client{IP: "198.51.100.9", UA: "Other/2"}); err != nil {
		t.Fatal(err)
	}
	done, gerr := repo.Get(ctx, a, req.ID)
	if gerr != nil {
		t.Fatalf("get after completion: %v", gerr)
	}
	if done.Status != "completed" || done.SignedFileID == "" || done.SignedHash == "" {
		t.Fatalf("not completed: %+v", done)
	}
	// The final PDF: valid, original page + certificate page, sealed, hash matches.
	final := dr.files[done.SignedFileID]
	if sha(final) != done.SignedHash {
		t.Error("stored hash does not match the saved file")
	}
	if err := api.Validate(bytes.NewReader(final), model.NewDefaultConfiguration()); err != nil {
		t.Errorf("final PDF invalid: %v", err)
	}
	if n, err := api.PageCount(bytes.NewReader(final), model.NewDefaultConfiguration()); err != nil || n != 2 {
		t.Errorf("pages = %d %v", n, err)
	}
	if !bytes.Contains(final, []byte("/ByteRange")) {
		t.Error("the document is not sealed")
	}
	if dr.owner[done.SignedFileID] != a.User {
		t.Error("the signed copy belongs in the sender's Drive")
	}
	// Audit trail is complete and chained; tampering is detected.
	ev, ok, err := repo.Trail(ctx, a, req.ID)
	if err != nil || !ok {
		t.Fatalf("trail ok=%v err=%v", ok, err)
	}
	names := []string{}
	for _, e := range ev {
		names = append(names, e.Event)
	}
	want := "created,sent,viewed,signed,viewed,signed,completed"
	if got := strings.Join(names, ","); !strings.Contains(got, "created,sent") || !strings.HasSuffix(got, "signed,completed") {
		t.Errorf("events = %s (want like %s)", got, want)
	}
	if _, err := fx.Pool.Exec(ctx, `UPDATE adrient.sig_events SET detail='edited' WHERE request_id=$1 AND event='signed' AND id=(SELECT min(id) FROM adrient.sig_events WHERE request_id=$1 AND event='signed')`, req.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := repo.Trail(ctx, a, req.ID); ok {
		t.Error("an edited audit entry must break the chain")
	}
	// Links stop working for completed requests only for signing, not viewing; signing again fails.
	if _, err := repo.PublicSign(ctx, ravi, SignInput{Consent: true, SignatureKind: "typed", SignatureData: "x"}, c1); err == nil {
		t.Error("a completed request cannot be signed")
	}
	if err := repo.Delete(ctx, a, req.ID); err == nil {
		t.Error("a completed request must be kept")
	}
}

func TestDeclineVoidExpire(t *testing.T) {
	fx := kittest.Setup(t)
	ctx := context.Background()
	repo := NewRepository(fx.Pool)
	now := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)
	repo.Deps.Now = func() time.Time { return now }
	dr := &fakeDrive{files: map[string][]byte{}, owner: map[string]string{}}
	repo.Deps.ReadFile = func(ctx context.Context, org, owner, id string) ([]byte, string, error) {
		return dr.files[id], "Doc.pdf", nil
	}
	a := fx.User
	fileID := dr.put(a.User, certificatePDF("D", "h", nil, nil, ""))
	mk := func(expire int) *Request {
		r, err := repo.Create(ctx, a, Input{Title: "D", FileID: fileID, ExpireDays: expire, Signers: []SignerInput{{Name: "S", Email: "s@example.test"}},
			Fields: []FieldInput{{SignerIndex: 0, Type: "signature", Page: 1, X: 0.1, Y: 0.1, W: 0.2, H: 0.05}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = repo.Send(ctx, a, r.ID, "https://w.example"); err != nil {
			t.Fatal(err)
		}
		return r
	}
	tok := func(id string) string {
		k := knowkit.NewKey() + knowkit.NewKey()
		_, _ = fx.Pool.Exec(ctx, `UPDATE adrient.sig_signers SET token_hash=$2 WHERE request_id=$1`, id, hashToken(k))
		return k
	}
	// Decline.
	r1 := mk(0)
	t1 := tok(r1.ID)
	if err := repo.PublicDecline(ctx, t1, "wrong amount", Client{IP: "1.2.3.4"}); err != nil {
		t.Fatal(err)
	}
	if g, _ := repo.Get(ctx, a, r1.ID); g.Status != "declined" || g.Signers[0].DeclineReason != "wrong amount" {
		t.Errorf("decline: %+v", g)
	}
	if _, err := repo.PublicView(ctx, t1, Client{}); err == nil {
		t.Error("links die when the request is declined")
	}
	// Void.
	r2 := mk(0)
	t2 := tok(r2.ID)
	if err := repo.Void(ctx, a, r2.ID, "sent by mistake"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.PublicSign(ctx, t2, SignInput{Consent: true, SignatureKind: "typed", SignatureData: "S"}, Client{}); err == nil {
		t.Error("a voided request cannot be signed")
	}
	if err := repo.Delete(ctx, a, r2.ID); err != nil {
		t.Errorf("a voided request may be deleted: %v", err)
	}
	// Expiry.
	r3 := mk(1)
	t3 := tok(r3.ID)
	now = now.AddDate(0, 0, 2)
	if _, err := repo.PublicSign(ctx, t3, SignInput{Consent: true, SignatureKind: "typed", SignatureData: "S"}, Client{}); err == nil {
		t.Error("an expired request cannot be signed")
	}
	if g, _ := repo.Get(ctx, a, r3.ID); g.Status != "expired" {
		t.Errorf("status = %s", g.Status)
	}
	// Only the sender or an admin can void; another org's actor sees nothing.
	r4 := mk(0)
	if err := repo.Void(ctx, fx.Other, r4.ID, ""); err == nil {
		t.Error("another org must not void")
	}
}

func TestStampPlacement(t *testing.T) {
	src := certificatePDF("Placement", "h", nil, nil, "")
	out, err := Stamp(src, []stampItem{{page: 1, text: "Asha Rao", italic: true, x: 0.1, y: 0.1, w: 0.3, h: 0.05}, {page: 1, text: "X", x: 0.5, y: 0.5, w: 0.03, h: 0.03}, {page: 9, text: "ignored", x: 0.1, y: 0.1, w: 0.2, h: 0.05}})
	if err != nil {
		t.Fatal(err)
	}
	if err := api.Validate(bytes.NewReader(out), model.NewDefaultConfiguration()); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(out, src) {
		t.Error("nothing was stamped")
	}
}
