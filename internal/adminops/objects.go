// Package adminops implements the operator tooling behind the adrient-admin
// CLI: backup, restore, per-org export and health checks. It is plain Go over
// pgx and an S3-compatible object store, with no dependency on pg_dump.
package adminops

import (
	"context"
	"io"
	"time"

	"github.com/Adrient-tech/Adrient-Workspace/internal/drive"
)

// ObjectStore is the slice of the blob store the tooling needs.
type ObjectStore interface {
	List(ctx context.Context, prefix string, fn func(key string, size int64) error) error
	Get(ctx context.Context, key string) (body io.ReadCloser, mime string, size int64, err error)
	Put(ctx context.Context, key, mime string, size int64, body io.Reader) error
}

// BlobStore adapts drive.Blobs to ObjectStore.
type BlobStore struct{ B *drive.Blobs }

func (s BlobStore) List(ctx context.Context, prefix string, fn func(string, int64) error) error {
	return s.B.ListObjects(ctx, prefix, func(k string, sz int64, _ time.Time) error { return fn(k, sz) })
}
func (s BlobStore) Get(ctx context.Context, key string) (io.ReadCloser, string, int64, error) {
	return s.B.Get(ctx, key)
}
func (s BlobStore) Put(ctx context.Context, key, mime string, size int64, body io.Reader) error {
	return s.B.Put(ctx, key, mime, size, body)
}
