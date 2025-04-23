package scaleway

import (
	"context"
	"errors"
	"fmt"

	file "github.com/scaleway/scaleway-sdk-go/api/file/v1alpha1"
	"github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// Client is a wrapper over the scaleway-sdk-go client.
type Client interface {
	AttachFileSystem(ctx context.Context, serverID string, fsID string, zone scw.Zone) error
	CreateFileSystem(ctx context.Context, name string, size int64, region scw.Region) (*file.FileSystem, error)
	DeleteFileSystem(ctx context.Context, id string, region scw.Region) error
	DetachFileSystem(ctx context.Context, serverID string, fsID string, zone scw.Zone) error
	ExpandFileSystem(ctx context.Context, id string, region scw.Region, size int64) (*file.FileSystem, error)
	GetFileSystem(ctx context.Context, id string, region scw.Region) (*file.FileSystem, error)
	GetFileSystemByName(ctx context.Context, name string, region scw.Region) (*file.FileSystem, error)
}

// client is a wrapper over the scaleway-sdk-go client.
type client struct {
	file     *file.API
	instance *instance.API
}

// NewClient creates a new Scaleway Client. It uses environment variables to
// initialize. The following environment variables must be set:
//   - SCW_DEFAULT_REGION
//   - SCW_DEFAULT_ZONE
//   - SCW_DEFAULT_PROJECT_ID
//   - SCW_ACCESS_KEY
//   - SCW_SECRET_KEY
func NewClient(userAgent string) (Client, error) {
	c, err := scw.NewClient(scw.WithEnv(), scw.WithUserAgent(userAgent))
	if err != nil {
		return nil, fmt.Errorf("failed to create Scaleway client: %w", err)
	}

	if _, ok := c.GetDefaultRegion(); !ok {
		return nil, errors.New("Scaleway default region must be set")
	}

	if _, ok := c.GetDefaultProjectID(); !ok {
		return nil, errors.New("Scaleway default project ID must be set")
	}

	if _, ok := c.GetDefaultZone(); !ok {
		return nil, errors.New("Scaleway default zone must be set")
	}

	return &client{
		file:     file.NewAPI(c),
		instance: instance.NewAPI(c),
	}, nil
}

// UnauthenticatedClient is a wrapper over the scaleway-sdk-go client to access
// unauthenticated methods.
type UnauthenticatedClient interface {
	GetServerType(ctx context.Context, name string, zone scw.Zone) (*instance.ServerType, error)
}

// unauthenticatedClient is a wrapper over the scaleway-sdk-go client to access
// unauthenticated methods.
type unauthenticatedClient struct {
	instance *instance.API
}

// NewUnauthenticatedClient creates a new UnauthenticatedClient.
func NewUnauthenticatedClient(userAgent string) (UnauthenticatedClient, error) {
	c, err := scw.NewClient(scw.WithoutAuth(), scw.WithEnv())
	if err != nil {
		return nil, fmt.Errorf("failed to create unauthenticated Scaleway client: %w", err)
	}

	return &unauthenticatedClient{
		instance: instance.NewAPI(c),
	}, nil
}
