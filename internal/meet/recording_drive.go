package meet

import (
	"context"

	"github.com/Adrient-tech/Adrient-Workspace/internal/drive"
)

// DriveAdapter adapts drive.Repository to DriveFiles.
type DriveAdapter struct{ Repo *drive.Repository }

func (d DriveAdapter) NewKey() (string, error) { return drive.NewStorageKey() }

// Folder finds (or creates) the owner's root-level folder called name.
func (d DriveAdapter) Folder(ctx context.Context, orgID, ownerID, name string) (string, error) {
	tok := ""
	for {
		fs, next, err := d.Repo.ListChildren(ctx, orgID, "", false, 200, tok)
		if err != nil {
			return "", err
		}
		for _, f := range fs {
			if f.MimeType == drive.FolderMimeType && f.Name == name && f.OwnerID == ownerID {
				return f.ID, nil
			}
		}
		if next == "" {
			break
		}
		tok = next
	}
	f, err := d.Repo.CreateFolder(ctx, orgID, ownerID, "", name)
	if err != nil {
		return "", err
	}
	return f.ID, nil
}

func (d DriveAdapter) AddFile(ctx context.Context, orgID, ownerID, parent, name, mime, key string, size int64) (string, error) {
	f, err := d.Repo.CreateFile(ctx, orgID, ownerID, parent, name, mime, key, size)
	if err != nil {
		return "", err
	}
	return f.ID, nil
}
