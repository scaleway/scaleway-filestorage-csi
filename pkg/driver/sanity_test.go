package driver

import (
	"fmt"
	"os"
	"testing"

	"github.com/kubernetes-csi/csi-test/v5/pkg/sanity"
	"github.com/scaleway/scaleway-filestorage-csi/pkg/scaleway"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"golang.org/x/sync/singleflight"
	"k8s.io/mount-utils"
)

func TestSanityCSI(t *testing.T) {
	var (
		zone           = scw.ZoneFrPar1
		region         = scw.RegionFrPar
		endpoint       = "/tmp/csi-testing.sock"
		serverID       = "fb094b6a-a732-4d5f-8283-bd6726ff5938"
		maxFileSystems = int64(4)
	)

	driver := &Driver{
		config: &DriverConfig{
			Endpoint: fmt.Sprintf("unix://%s", endpoint),
			Mode:     AllMode,
		},
		controllerService: &controllerService{
			scaleway: scaleway.NewFakeClient(region, serverID, maxFileSystems),
			sf:       &singleflight.Group{},
		},
		nodeService: &nodeService{
			nodeID:            serverID,
			nodeZone:          zone,
			nodeRegion:        region,
			maxVolumesPerNode: maxFileSystems,
			mounter:           mount.NewFakeMounter(nil),
		},
	}

	go driver.Run() //nolint:errcheck // an error here would fail the test anyway since the grpc server would not be started

	config := sanity.NewTestConfig()
	config.Address = endpoint
	config.TestNodeVolumeAttachLimit = true
	config.TestVolumeSize = MinVolumeSize
	config.TestVolumeExpandSize = config.TestVolumeSize * 2
	config.RemoveTargetPath = func(path string) error {
		return os.RemoveAll(path) //nolint: wrapcheck
	}
	config.RemoveStagingPath = func(path string) error {
		return os.RemoveAll(path) //nolint: wrapcheck
	}
	sanity.Test(t, config)
	driver.srv.GracefulStop()
	os.RemoveAll(endpoint)
}
