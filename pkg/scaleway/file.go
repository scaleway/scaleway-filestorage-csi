package scaleway

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	file "github.com/scaleway/scaleway-sdk-go/api/file/v1alpha1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

const (
	waitForTerminalStatusTimeout    = time.Minute
	waitForTerminalStatusRetryDelay = time.Second
)

var fileSystemTerminalStatuses = []file.FileSystemStatus{
	file.FileSystemStatusAvailable,
	file.FileSystemStatusError,
}

// CreateFileSystem creates a new FileSystem with the provided settings and waits
// for the FileSystem to be in a terminal status.
func (c *client) CreateFileSystem(ctx context.Context, name string, size int64, region scw.Region) (*file.FileSystem, error) {
	fs, err := c.file.CreateFileSystem(&file.CreateFileSystemRequest{
		Name:   name,
		Size:   uint64(size),
		Region: region,
	}, scw.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to create FileSystem: %w", err)
	}

	fs, err = c.waitForFileSystem(ctx, fs)
	if err != nil {
		return nil, fmt.Errorf("error waiting for FileSystem to be in terminal state: %w", err)
	}

	return fs, nil
}

// GetFileSystemByName searches for an existing FileSystem by its name. A ResourceNotFoundError
// is returned if no existing FileSystem matches the provided name.
func (c *client) GetFileSystemByName(ctx context.Context, name string, region scw.Region) (*file.FileSystem, error) {
	listFS, err := c.file.ListFileSystems(&file.ListFileSystemsRequest{
		Region: region,
		Name:   scw.StringPtr(name),
	}, scw.WithAllPages(), scw.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to list FileSystems: %w", err)
	}

	for _, fs := range listFS.Filesystems {
		if fs.Name == name {
			return fs, nil
		}
	}

	return nil, &scw.ResourceNotFoundError{
		Resource:   "file_system",
		ResourceID: name,
	}
}

// GetFileSystem gets an existing FileSystem by its ID.
func (c *client) GetFileSystem(ctx context.Context, id string, region scw.Region) (*file.FileSystem, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, &scw.ResourceNotFoundError{
			Resource:   "file_system",
			ResourceID: id,
		}
	}

	fs, err := c.file.GetFileSystem(&file.GetFileSystemRequest{
		Region:       region,
		FilesystemID: id,
	}, scw.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to get FileSystem: %w", err)
	}

	return fs, nil
}

// ResizeFileSystem increases or decreases the size of an existing FileSystem and waits for the
// FileSystem to be in a terminal status.
func (c *client) ResizeFileSystem(ctx context.Context, id string, region scw.Region, size int64) (*file.FileSystem, error) {
	fs, err := c.file.UpdateFileSystem(&file.UpdateFileSystemRequest{
		Region:       region,
		FilesystemID: id,
		Size:         scw.Uint64Ptr(uint64(size)),
	}, scw.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to resize FileSystem: %w", err)
	}

	fs, err = c.waitForFileSystem(ctx, fs)
	if err != nil {
		return nil, fmt.Errorf("error waiting for FileSystem to be in terminal state: %w", err)
	}

	return fs, nil
}

// DeleteFileSystem deletes a FileSystem by ID.
func (c *client) DeleteFileSystem(ctx context.Context, id string, region scw.Region) error {
	if _, err := uuid.Parse(id); err != nil {
		return nil
	}

	if err := c.file.DeleteFileSystem(&file.DeleteFileSystemRequest{
		Region:       region,
		FilesystemID: id,
	}, scw.WithContext(ctx)); err != nil {
		return fmt.Errorf("failed to delete FileSystem: %w", err)
	}

	return nil
}

func (c *client) waitForFileSystem(ctx context.Context, fs *file.FileSystem) (*file.FileSystem, error) {
	ctx, cancel := context.WithTimeout(ctx, waitForTerminalStatusTimeout)
	defer cancel()

	for {
		if slices.Contains(fileSystemTerminalStatuses, fs.Status) {
			return fs, nil
		}

		select {
		case <-time.After(waitForTerminalStatusRetryDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		var err error
		fs, err = c.file.GetFileSystem(&file.GetFileSystemRequest{
			Region:       fs.Region,
			FilesystemID: fs.ID,
		}, scw.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("failed to get filesystem: %w", err)
		}
	}
}
