# PDF app: features

The PDF app is the workspace's PDF editor and document-signing service. It
runs inside the workspace server: the browser app is served under `/pdf/` and
its API under `/pdf-api/api/`, using the workspace sign-in. All editing and
signing runs in the browser and on our own Go server; no outside service is
called.

## Editor (`/pdf/editor`)

| Area | What it does |
|---|---|
| Open | Drag-drop or pick a PDF, start a blank PDF, reopen an editable copy (annotations restore as objects), drafts kept in the browser |
| Edit text in place | Click a line of existing text; it becomes an editable box with the same position, size, colour and style. When the file embeds the line's font and that font has every glyph of the new text, the new text is drawn in that font; otherwise in the closest standard font. On download our server removes the old glyphs from the page content (they are gone from the file, not covered) and the new text is drawn in their place |
| Add text | Text boxes with font family, size, bold/italic, colour, alignment, lists, line spacing, rotation; non-Latin text uses an embedded subset font |
| Images | Add, move, resize, rotate, replace and delete PNG/JPEG images |
| Signatures | Draw, type or upload a signature or initials; "Save for next time" keeps them in a per-user store on the server; place, resize and reuse them; date stamps |
| Digital signing | Documents sent for signing are signed with X.509 certificates (PAdES, PKCS#7 with optional RFC 3161 timestamp); see "Signing service" below |
| Annotations | Highlight, underline, strike-through, sticky notes with a comments sidebar, rectangles, rounded rectangles, ellipses, lines, arrows, polygons, polylines, freehand ink, eraser, whiteout, preset and custom stamps |
| Forms | Fill existing form fields; create text, checkbox, radio, dropdown, list box and button fields; field properties, calculations and validation; flatten on download |
| Pages | Insert blank pages, import pages from another PDF, delete, duplicate, rotate, reorder (drag thumbnails or move up/down), extract a range, delete a range, crop, page backgrounds, headers/footers and page numbers |
| Redaction | Mark areas; on download our server removes the glyphs, the vector drawings and the image pixels inside each area from the file. A page whose content cannot be fully rewritten (content inside form objects) is flattened to an image instead, so nothing under a mark survives |
| Search | Find text across pages |
| Download | Flattened PDF, editable copy, compressed copy (our server removes duplicate fonts and images and recompresses streams), export pages as PNG or JPEG |
| Layout | Desktop: tool sidebar, page thumbnails, comments sidebar. Phone: the tool panels open in a bottom sheet from a floating Tools button |

## Tools (`/pdf/tools`)

Compress or optimise a PDF (server-side optimisation, optional metadata
removal, or flatten pages to images), split by ranges or every N pages, and
compare the text of two PDFs.

## Signing service (`/pdf/documents`)

| Area | What it does |
|---|---|
| Documents | Upload or create, edit, prepare (signers, fields), send, void, delete; templates |
| Signers | Sequential or parallel order; guest signing by link, no account needed; decline with reason; reminders |
| Fields | Signature, initials, date and text fields placed per signer |
| Certificates | Self-signed CA created on first start (4096-bit RSA, 10 years); per-signer certificates (2048-bit RSA, 1 year); private keys encrypted with AES-256-GCM under a key-encryption key |
| Signatures | PAdES signatures embedded in the PDF, verifiable in desktop PDF readers; optional RFC 3161 timestamp from a configured authority; browser-extension and hardware-token signing (see `docs/X509_SIGNING.md`) |
| Verification | `GET /api/documents/:id/verify` re-checks the chain against the trust bundle, recomputes the hash and validates the stored signature |
| Audit trail | Every action with time, IP and user agent |
| Admin | Superadmins see all documents (metadata only) and manage the superadmin list |

## API (under `/pdf-api/api`)

Editor tools (stateless; the PDF travels in the request body):

```
POST /tools/remove-content?regions=[{page,x,y,w,h,glyphsOnly}]   remove glyphs, drawings and image pixels in areas (points, origin bottom-left)
POST /tools/page-fonts?page=N                                    fonts of a page with embedded TrueType/OpenType programs
POST /tools/compress                                             optimise and recompress
```

Saved signatures (per signed-in user):

```
GET    /user/signatures
POST   /user/signatures          {kind: "signature"|"initials", label, imagePng: "data:image/png;base64,..."}
DELETE /user/signatures/{id}
```

Documents and signing:

```
POST   /documents                         create
GET    /documents                         list (own documents)
GET    /documents/:id                     get with signers
PATCH  /documents/:id                     update
DELETE /documents/:id                     delete
POST   /documents/:id/send                send for signing
POST   /documents/:id/void                void
POST   /documents/:id/signers             add signer
DELETE /documents/:id/signers/:sid        remove signer
POST   /documents/:id/signers/:sid/resend send the invitation again
POST   /documents/:id/fields              add field
PATCH  /documents/:id/fields/:fid         move or resize field
DELETE /documents/:id/fields/:fid         remove field
GET    /documents/:id/completed           signed document URL
GET    /documents/:id/signatures          signature details
GET    /documents/:id/verify              verify signatures
GET    /documents/:id/audit               audit trail
GET    /documents/:id/annotations         author annotations
PUT    /documents/:id/annotations         save annotations
POST   /documents/:id/replace-url         upload URL for an edited copy
GET    /to-sign                           documents waiting for the caller
GET    /sign/:token                       guest signing session
GET    /sign/:token/annotations           author annotations for the signer
POST   /sign/:token/view                  record a view
POST   /sign/:token/submit                submit
POST   /sign/:token/decline               decline
GET    /admin/superadmins                 list superadmins
POST   /admin/superadmins/{email}         grant
DELETE /admin/superadmins/{email}         revoke
GET    /admin/documents                   all documents (metadata only)
```

## Data

Tables in the PDF app's own database: `documents` (with author annotations),
`signers`, `signature_fields`, `signatures`, `audit_trail`,
`signing_certificates`, `superadmins`, `templates`, `user_signatures`
(saved signatures and initials, keyed by the user's verified email).

## Configuration

The workspace server starts the PDF app in-process when
`ADRIENT_PDF_BUILTIN=true`, serving the built app from
`ADRIENT_PDF_STATIC_DIR`. The app itself reads `PDF_*` variables:
`PDF_DATABASE_URL`, `PDF_STORAGE_ENDPOINT` / `_BUCKET` / `_REGION` /
`_ACCESS_KEY` / `_SECRET_KEY` (S3-compatible storage),
`PDF_CRYPTO_KEY_ENCRYPTION_KEY` (base64, 32 bytes), optional
`PDF_CRYPTO_TSA_URL`, and the `PDF_EMAIL_SMTP_*` settings for invitations.
`deploy/local/stack.sh` sets these for the local stack.
