package meet

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// RecordingsFolder is the Drive folder (per host) that receives recordings.
const RecordingsFolder = "Meet recordings"

// BlobReader reads (and removes) the raw egress output in the recordings bucket.
type BlobReader interface {
	Get(ctx context.Context, key string) (io.ReadCloser, string, int64, error)
	Delete(ctx context.Context, key string) error
}

// BlobWriter stores the Drive copy.
type BlobWriter interface {
	Put(ctx context.Context, key, mimeType string, size int64, body io.Reader) error
}

// DriveFiles creates the Drive entries for a finished recording.
type DriveFiles interface {
	// Folder returns the id of the owner's root-level folder name, creating it if missing.
	Folder(ctx context.Context, orgID, ownerID, name string) (string, error)
	NewKey() (string, error)
	AddFile(ctx context.Context, orgID, ownerID, parent, name, mime, key string, size int64) (id string, err error)
}

// RecordingJob describes one recording being produced.
type RecordingJob struct {
	EgressID  string
	OrgID     string
	RoomID    string
	Code      string
	RoomName  string
	StartedBy string
	StarterNm string
	Key       string // object key in the egress bucket
	StartedAt time.Time
}

// Recorder copies finished recordings into Drive and tells people about them.
type Recorder struct {
	Src   BlobReader
	Dst   BlobWriter
	Drive DriveFiles
	// Notify adds a notification-center entry for userID.
	Notify func(ctx context.Context, orgID, userID, title, body, url string)
	// PostCard posts a message with the file link into a chat channel (as userID).
	PostCard func(ctx context.Context, orgID, channelID, userID, userName, body string) error
	// Channel resolves the chat channel a meeting code belongs to ("" = none).
	Channel func(ctx context.Context, orgID, code string) string
	// Saved records the outcome (status "saved"/"failed" and the Drive file id).
	Saved func(ctx context.Context, egressID, status, driveFileID string)
}

func recordingFileName(job RecordingJob) string {
	n := strings.TrimSpace(job.RoomName)
	if n == "" {
		n = "Meeting"
	}
	n = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r < 0x20 {
			return '-'
		}
		return r
	}, n)
	return fmt.Sprintf("%s - %s.mp4", n, job.StartedAt.Format("2006-01-02 15-04"))
}

// Finish copies the finished egress file into the host's Drive folder, then
// notifies the host and posts a file card in the originating channel.
func (rc *Recorder) Finish(ctx context.Context, job RecordingJob) error {
	fail := func(err error) error {
		if rc.Saved != nil {
			rc.Saved(ctx, job.EgressID, "failed", "")
		}
		return err
	}
	body, mime, size, err := rc.Src.Get(ctx, job.Key)
	if err != nil {
		return fail(fmt.Errorf("read recording: %w", err))
	}
	defer body.Close()
	if mime == "" || mime == "binary/octet-stream" {
		mime = "video/mp4"
	}
	key, err := rc.Drive.NewKey()
	if err != nil {
		return fail(err)
	}
	// The S3 client needs a seekable body to checksum: spool to a temp file.
	tmp, err := os.CreateTemp("", "meet-rec-*.mp4")
	if err != nil {
		return fail(err)
	}
	defer func() { _ = tmp.Close(); _ = os.Remove(tmp.Name()) }()
	n, err := io.Copy(tmp, body)
	if err != nil {
		return fail(fmt.Errorf("read recording: %w", err))
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	size = n
	if err := rc.Dst.Put(ctx, key, mime, size, tmp); err != nil {
		return fail(fmt.Errorf("copy recording: %w", err))
	}
	folder, err := rc.Drive.Folder(ctx, job.OrgID, job.StartedBy, RecordingsFolder)
	if err != nil {
		return fail(fmt.Errorf("recordings folder: %w", err))
	}
	name := recordingFileName(job)
	id, err := rc.Drive.AddFile(ctx, job.OrgID, job.StartedBy, folder, name, mime, key, size)
	if err != nil {
		return fail(fmt.Errorf("create drive file: %w", err))
	}
	_ = rc.Src.Delete(ctx, job.Key)
	if rc.Saved != nil {
		rc.Saved(ctx, job.EgressID, "saved", id)
	}
	link := "/drive/file/" + id
	if rc.Notify != nil {
		rc.Notify(ctx, job.OrgID, job.StartedBy, "Meeting recording saved", name+" is in Drive > "+RecordingsFolder, link)
	}
	if rc.Channel != nil && rc.PostCard != nil {
		if ch := rc.Channel(ctx, job.OrgID, job.Code); ch != "" {
			msg := fmt.Sprintf("🎥 **Meeting recording** by %s: [%s](%s)", job.StarterNm, name, link)
			if err := rc.PostCard(ctx, job.OrgID, ch, job.StartedBy, job.StarterNm, msg); err != nil {
				slog.Warn("meet: post recording card", "err", err)
			}
		}
	}
	return nil
}

// watchEgress polls until the egress finishes, then runs Finish. It gives up
// after maxWait. Safe to call once per recording.
func (h *SFUHandler) watchEgress(name string, job RecordingJob) {
	if h.Rec == nil || h.admin == nil {
		return
	}
	go func() {
		deadline := h.now().Add(h.recWait)
		for h.now().Before(deadline) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			done, ok, err := h.admin.EgressStatus(ctx, job.EgressID)
			cancel()
			if err == nil && done {
				h.clearEgress(name, job.RoomID, job.EgressID)
				fctx, fcancel := context.WithTimeout(context.Background(), 10*time.Minute)
				defer fcancel()
				if !ok {
					if h.RecDB != nil {
						_ = h.RecDB.FinishRecording(fctx, job.EgressID, "failed", "")
					}
					return
				}
				if ferr := h.Rec.Finish(fctx, job); ferr != nil {
					slog.Warn("meet: finish recording", "egress", job.EgressID, "err", ferr)
				}
				return
			}
			time.Sleep(h.recPoll)
		}
	}()
}
