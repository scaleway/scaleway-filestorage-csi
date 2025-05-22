package scaleway

import (
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/google/uuid"
	file "github.com/scaleway/scaleway-sdk-go/api/file/v1alpha1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

var _ Client = &FakeClient{}

type FakeClient struct {
	serversAttachedFileSystems map[string][]string
	fileSystems                map[string]*file.FileSystem
	defaultRegion              scw.Region
	maxFileSystems             int64
	mu                         sync.Mutex
}

func NewFakeClient(defaultRegion scw.Region, serverID string, maxFileSystems int64) *FakeClient {
	return &FakeClient{
		serversAttachedFileSystems: map[string][]string{serverID: nil},
		fileSystems:                make(map[string]*file.FileSystem),
		defaultRegion:              defaultRegion,
		maxFileSystems:             maxFileSystems,
	}
}

func (f *FakeClient) getRegion(region scw.Region) scw.Region {
	if region == "" {
		return f.defaultRegion
	}

	return region
}

func (f *FakeClient) AttachFileSystem(ctx context.Context, serverID string, fsID string, zone scw.Zone) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	serverAttachedFileSystems, ok := f.serversAttachedFileSystems[serverID]
	if !ok {
		return &scw.ResourceNotFoundError{
			Resource:   "server",
			ResourceID: serverID,
		}
	}

	if _, ok := f.fileSystems[fsID]; !ok {
		return &scw.ResourceNotFoundError{
			Resource:   "file_system",
			ResourceID: fsID,
		}
	}

	if !slices.Contains(serverAttachedFileSystems, fsID) {
		if len(serverAttachedFileSystems) == int(f.maxFileSystems) {
			return &scw.PreconditionFailedError{
				Precondition: "MaxFileSystems limit reached",
			}
		}

		serverAttachedFileSystems = append(serverAttachedFileSystems, fsID)
	}

	f.serversAttachedFileSystems[serverID] = serverAttachedFileSystems
	return nil
}

func (f *FakeClient) CreateFileSystem(ctx context.Context, name string, size int64, region scw.Region) (*file.FileSystem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, fs := range f.fileSystems {
		if fs.Name == name {
			return fs, nil
		}
	}

	fs := &file.FileSystem{
		ID:     uuid.NewString(),
		Name:   name,
		Size:   scw.Size(size),
		Status: file.FileSystemStatusAvailable,
		Region: f.getRegion(region),
	}

	f.fileSystems[fs.ID] = fs

	return fs, nil
}

func (f *FakeClient) DeleteFileSystem(ctx context.Context, id string, region scw.Region) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if _, ok := f.fileSystems[id]; !ok {
		return &scw.ResourceNotFoundError{
			Resource:   "file_system",
			ResourceID: id,
		}
	}

	// Do not allow FileSystem deletion if it's still attached to a server.
	for serverID, attachments := range f.serversAttachedFileSystems {
		if slices.Contains(attachments, id) {
			return &scw.PreconditionFailedError{
				Precondition: fmt.Sprintf("FileSystem %s is still attached to server %s", id, serverID),
			}
		}
	}

	delete(f.fileSystems, id)

	return nil
}

func (f *FakeClient) DetachFileSystem(ctx context.Context, serverID string, fsID string, zone scw.Zone) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	serverAttachedFileSystems, ok := f.serversAttachedFileSystems[serverID]
	if !ok {
		return &scw.ResourceNotFoundError{
			Resource:   "server",
			ResourceID: serverID,
		}
	}

	idx := slices.Index(serverAttachedFileSystems, fsID)
	if idx == -1 {
		return nil
	}

	f.serversAttachedFileSystems[serverID] = slices.Delete(serverAttachedFileSystems, idx, idx+1)

	return nil
}

func (f *FakeClient) ResizeFileSystem(ctx context.Context, id string, region scw.Region, size int64) (*file.FileSystem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	fs, ok := f.fileSystems[id]
	if !ok {
		return nil, &scw.ResourceNotFoundError{
			Resource:   "file_system",
			ResourceID: id,
		}
	}

	fs.Size = scw.Size(size)

	f.fileSystems[id] = fs

	return fs, nil
}

func (f *FakeClient) GetFileSystem(ctx context.Context, id string, region scw.Region) (*file.FileSystem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	fs, ok := f.fileSystems[id]
	if !ok {
		return nil, &scw.ResourceNotFoundError{
			Resource:   "file_system",
			ResourceID: id,
		}
	}

	return fs, nil
}

func (f *FakeClient) GetFileSystemByName(ctx context.Context, name string, region scw.Region) (*file.FileSystem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, fs := range f.fileSystems {
		if fs.Name == name {
			return fs, nil
		}
	}

	return nil, &scw.ResourceNotFoundError{
		Resource:   "file_system",
		ResourceID: name,
	}
}
