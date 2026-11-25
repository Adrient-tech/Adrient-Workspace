package cloudimport

import (
	"bufio"
	"bytes"
	"io"
	"strings"
)

const maxMboxMessage = 50 << 20

// SplitMbox calls fn with each message of an mboxrd/mboxo stream. "From "
// separator lines are dropped and ">From " quoting is undone. Messages larger
// than 50 MB are skipped.
func SplitMbox(r io.Reader, fn func(raw []byte) error) error {
	br := bufio.NewReaderSize(r, 1<<20)
	var cur bytes.Buffer
	started, tooBig := false, false
	flush := func() error {
		if started && !tooBig && cur.Len() > 0 {
			b := make([]byte, cur.Len())
			copy(b, cur.Bytes())
			if err := fn(b); err != nil {
				return err
			}
		}
		cur.Reset()
		tooBig = false
		return nil
	}
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			if bytes.HasPrefix(line, []byte("From ")) {
				if ferr := flush(); ferr != nil {
					return ferr
				}
				started = true
			} else if started {
				if l := bytes.TrimLeft(line, ">"); len(l) < len(line) && bytes.HasPrefix(l, []byte("From ")) {
					line = line[1:]
				}
				if cur.Len()+len(line) > maxMboxMessage {
					tooBig = true
				} else if !tooBig {
					cur.Write(line)
				}
			}
		}
		if err == io.EOF {
			return flush()
		}
		if err != nil {
			return err
		}
	}
}

// mboxGmailLabels reads the X-Gmail-Labels header Takeout adds.
func mboxGmailLabels(raw []byte) []string {
	hdrEnd := bytes.Index(raw, []byte("\r\n\r\n"))
	if hdrEnd < 0 {
		hdrEnd = bytes.Index(raw, []byte("\n\n"))
	}
	if hdrEnd < 0 {
		hdrEnd = len(raw)
	}
	var val string
	lines := strings.Split(strings.ReplaceAll(string(raw[:hdrEnd]), "\r\n", "\n"), "\n")
	for i, l := range lines {
		if strings.HasPrefix(strings.ToLower(l), "x-gmail-labels:") {
			val = strings.TrimSpace(l[len("x-gmail-labels:"):])
			for j := i + 1; j < len(lines) && (strings.HasPrefix(lines[j], " ") || strings.HasPrefix(lines[j], "\t")); j++ {
				val += strings.TrimSpace(lines[j])
			}
			break
		}
	}
	if val == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(val, ",") {
		if p = strings.Trim(strings.TrimSpace(p), `"`); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// takeoutFolder turns Gmail labels into a folder + user labels. System labels
// (Inbox, Sent, Opened, Unread, Category ...) are not kept as labels.
func takeoutFolder(labels []string) (string, []string) {
	folder := "Archive"
	var user []string
	for _, l := range labels {
		switch strings.ToLower(l) {
		case "inbox":
			folder = "INBOX"
		case "sent":
			if folder == "Archive" {
				folder = "Sent"
			}
		case "drafts", "draft":
			folder = "Drafts"
		case "spam":
			folder = "Spam"
		case "trash":
			folder = "Trash"
		case "opened", "unread", "important", "starred", "archived", "chat":
		default:
			if strings.HasPrefix(strings.ToLower(l), "category ") {
				continue
			}
			user = append(user, l)
		}
	}
	return folder, user
}
