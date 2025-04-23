package scaleway

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

func (c *client) AttachFileSystem(ctx context.Context, serverID, fsID string, zone scw.Zone) error {
	if _, err := uuid.Parse(serverID); err != nil {
		return &scw.ResourceNotFoundError{
			Resource:   "server",
			ResourceID: serverID,
		}
	}

	if _, err := uuid.Parse(fsID); err != nil {
		return &scw.ResourceNotFoundError{
			Resource:   "file_system",
			ResourceID: fsID,
		}
	}

	if _, err := c.instance.AttachServerFileSystem(&instance.AttachServerFileSystemRequest{
		Zone:         zone,
		ServerID:     serverID,
		FilesystemID: fsID,
	}, scw.WithContext(ctx)); err != nil {
		return fmt.Errorf("failed to attach FileSystem %q to server %q: %w", fsID, serverID, err)
	}

	// TODO: Wait for FileSystem attachment. The status of the attachment is currently
	// not exposed in the Scaleway SDK, so we use time.Sleep for now.
	time.Sleep(3 * time.Second)

	return nil
}

func (c *client) DetachFileSystem(ctx context.Context, serverID, fsID string, zone scw.Zone) error {
	if _, err := uuid.Parse(serverID); err != nil {
		return &scw.ResourceNotFoundError{
			Resource:   "server",
			ResourceID: serverID,
		}
	}

	if _, err := uuid.Parse(fsID); err != nil {
		return &scw.ResourceNotFoundError{
			Resource:   "file_system",
			ResourceID: fsID,
		}
	}

	if _, err := c.instance.DetachServerFileSystem(&instance.DetachServerFileSystemRequest{
		Zone:         zone,
		ServerID:     serverID,
		FilesystemID: fsID,
	}, scw.WithContext(ctx)); err != nil {
		return fmt.Errorf("failed to detach FileSystem %q from server %q: %w", fsID, serverID, err)
	}

	return nil
}

func (c *unauthenticatedClient) GetServerType(ctx context.Context, name string, zone scw.Zone) (*instance.ServerType, error) {
	resp, err := c.instance.ListServersTypes(
		&instance.ListServersTypesRequest{Zone: zone},
		scw.WithAllPages(),
		scw.WithContext(ctx),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list server types: %w", err)
	}

	serverType, ok := resp.Servers[name]
	if !ok {
		return nil, fmt.Errorf("could not find server type %q", name)
	}

	return serverType, nil
}
