package driver

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/scaleway/scaleway-filestorage-csi/pkg/scaleway"
	file "github.com/scaleway/scaleway-sdk-go/api/file/v1alpha1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

const MinVolumeSize = 100000000000 // 100GB.

var (
	// controllerCapabilities represents the capabilites of the controller.
	controllerCapabilities = []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
	}

	// supportedAccessModes represents the supported access modes for the Scaleway FileSystems.
	supportedAccessModes = []csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY,
		csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_SINGLE_WRITER,
		csi.VolumeCapability_AccessMode_SINGLE_NODE_MULTI_WRITER,
	}
)

// controllerService implements csi.ControllerServer.
type controllerService struct {
	csi.UnimplementedControllerServer

	sf                 *singleflight.Group
	scaleway           scaleway.Client
	createVolumePrefix string
}

func newControllerService(config *DriverConfig) (*controllerService, error) {
	client, err := scaleway.NewClient(UserAgent())
	if err != nil {
		return nil, err
	}

	return &controllerService{
		sf:                 &singleflight.Group{},
		createVolumePrefix: config.Prefix,
		scaleway:           client,
	}, nil
}

// CreateVolume creates a new volume with the given CreateVolumeRequest.
// This function is idempotent
func (d *controllerService) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.V(4).Infof("CreateVolume called with request %+v", req)

	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "name not provided")
	}

	if err := validateVolumeCapabilities(req.GetVolumeCapabilities(), false); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "volumeCapabilities not supported: %s", err)
	}

	if req.GetVolumeContentSource() != nil {
		return nil, status.Errorf(codes.InvalidArgument, "volumeContentSource is not supported")
	}

	fsName := d.createVolumePrefix + req.GetName()

	size, err := getVolumeRequestCapacity(req.GetCapacityRange())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "capacityRange invalid: %s", err)
	}

	region, err := pickRegion(req.AccessibilityRequirements)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "accessibilityRequirements invalid: %s", err)
	}

	fs, err, _ := d.sf.Do(fmt.Sprintf("%s/%s", region, fsName), func() (any, error) {
		fs, err := d.scaleway.GetFileSystemByName(ctx, fsName, region)
		if err != nil && !scaleway.IsNotFoundError(err) {
			return nil, status.Errorf(codes.Internal, "failed to get FileSystem by name: %s", err)
		}

		if fs == nil {
			fs, err = d.scaleway.CreateFileSystem(ctx, fsName, size, region)
			if err != nil {
				return nil, status.Errorf(codes.Internal, "failed to create FileSystem: %s", err)
			}
		}

		if int64(fs.Size) != size {
			return nil, status.Errorf(codes.AlreadyExists, "volume already exists with a different capacity")
		}

		if fs.Status == file.FileSystemStatusCreating {
			msg := fmt.Sprintf("FileSystem %s not ready, current state: %s", fsName, fs.Status)
			klog.V(4).Info(msg)
			return nil, status.Error(codes.DeadlineExceeded, msg)
		}

		if fs.Status != file.FileSystemStatusAvailable {
			msg := fmt.Sprintf("FileSystem %s not ready, current state: %s", fsName, fs.Status)
			klog.V(4).Info(msg)
			return nil, status.Error(codes.Unavailable, msg)
		}

		return fs, nil
	})
	if err != nil {
		return nil, err
	}

	return &csi.CreateVolumeResponse{
		Volume: csiVolume(fs.(*file.FileSystem)),
	}, nil
}

// DeleteVolume deprovisions a volume.
// This operation MUST be idempotent.
func (d *controllerService) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.V(4).Infof("DeleteVolume called with request %+v", req)

	id, region, err := extractIDAndRegion(req.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	err = d.scaleway.DeleteFileSystem(ctx, id, region)
	if err != nil && !scaleway.IsNotFoundError(err) {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume perform the work that is necessary for making the volume available on the given node.
// This operation MUST be idempotent.
func (d *controllerService) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	klog.V(4).Infof("ControllerPublishVolume called with request %+v", req)

	fsID, _, err := extractIDAndRegion(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unable to extract fs ID and region: %s", err)
	}

	serverID, serverZone, err := extractIDAndZone(req.NodeId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unable to extract server ID and zone: %s", err)
	}

	if err := validateVolumeCapabilities([]*csi.VolumeCapability{req.GetVolumeCapability()}, false); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "volumeCapability not supported: %s", err)
	}

	if err := d.scaleway.AttachFileSystem(ctx, serverID, fsID, serverZone); err != nil {
		return nil, status.Error(codeFromScalewayError(err), err.Error())
	}

	return &csi.ControllerPublishVolumeResponse{}, nil
}

// ControllerUnpublishVolume is the reverse operation of ControllerPublishVolume
// This operation MUST be idempotent.
func (d *controllerService) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	klog.V(4).Infof("ControllerUnpublishVolume called with request %+v", req)

	fsID, _, err := extractIDAndRegion(req.GetVolumeId())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unable to extract fs ID and region: %s", err)
	}

	serverID, serverZone, err := extractIDAndZone(req.NodeId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unable to extract server ID and zone: %s", err)
	}

	if err := d.scaleway.DetachFileSystem(ctx, serverID, fsID, serverZone); err != nil {
		code := codeFromScalewayError(err)
		if code == codes.NotFound {
			// Ignore server not found error.
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, status.Error(code, err.Error())
	}

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ValidateVolumeCapabilities check if a pre-provisioned volume has all the capabilities
// that the CO wants. This RPC call SHALL return confirmed only if all the
// volume capabilities specified in the request are supported.
// This operation MUST be idempotent.
func (d *controllerService) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	klog.V(4).Infof("ValidateVolumeCapabilities called with request %+v", req)

	id, region, err := extractIDAndRegion(req.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	if _, err = d.scaleway.GetFileSystem(ctx, id, region); err != nil {
		return nil, status.Error(codeFromScalewayError(err), err.Error())
	}

	if err := validateVolumeCapabilities(req.GetVolumeCapabilities(), false); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "unsupported capabilities: %s", err)
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}, nil
}

// ControllerGetCapabilities returns the supported capabilities of controller service provided by the Plugin.
func (d *controllerService) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	klog.V(4).Infof("ControllerGetCapabilities called with request %+v", req)

	capabilities := make([]*csi.ControllerServiceCapability, 0, len(controllerCapabilities))
	for _, capability := range controllerCapabilities {
		capabilities = append(capabilities, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: capability,
				},
			},
		})
	}

	return &csi.ControllerGetCapabilitiesResponse{Capabilities: capabilities}, nil
}

// ControllerExpandVolume expands the given volume
func (d *controllerService) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	klog.V(4).Infof("ControllerExpandVolume called with request %+v", req)

	id, region, err := extractIDAndRegion(req.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	size, err := getVolumeRequestCapacity(req.GetCapacityRange())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "capacityRange invalid: %s", err)
	}

	fs, err, _ := d.sf.Do(req.GetVolumeId(), func() (any, error) {
		fs, err := d.scaleway.GetFileSystem(ctx, id, region)
		if err != nil {
			return nil, status.Error(codeFromScalewayError(err), err.Error())
		}

		// Do nothing if FS already has the expected size.
		if fs.Size >= scw.Size(size) {
			return fs, nil
		}

		fs, err = d.scaleway.ResizeFileSystem(ctx, id, region, size)
		if err != nil {
			return nil, status.Error(codeFromScalewayError(err), err.Error())
		}

		return fs, nil
	})
	if err != nil {
		return nil, err
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes:         int64(fs.(*file.FileSystem).Size),
		NodeExpansionRequired: false,
	}, nil
}

// getVolumeRequestCapacity returns the volume capacity that will be requested
// to the Scaleway block storage according to the provided capacity range.
func getVolumeRequestCapacity(capacityRange *csi.CapacityRange) (int64, error) {
	if capacityRange == nil {
		return MinVolumeSize, nil
	}

	requiredBytes := capacityRange.GetRequiredBytes()
	requiredBytesSet := requiredBytes > 0

	limitBytes := capacityRange.GetLimitBytes()
	limitBytesSet := limitBytes > 0

	if !requiredBytesSet && !limitBytesSet {
		return MinVolumeSize, nil
	}

	if requiredBytesSet && limitBytesSet && limitBytes < requiredBytes {
		return 0, errors.New("limit size is less than required size")
	}

	if requiredBytesSet && !limitBytesSet && requiredBytes < MinVolumeSize {
		return 0, errors.New("required size is less than the minimum size")
	}

	if limitBytesSet && limitBytes < MinVolumeSize {
		return 0, errors.New("limit size is less than the minimum size")
	}

	if requiredBytesSet && limitBytesSet && requiredBytes == limitBytes {
		return requiredBytes, nil
	}

	if requiredBytesSet {
		return requiredBytes, nil
	}

	if limitBytesSet {
		return limitBytes, nil
	}

	return MinVolumeSize, nil
}

// pickRegion returns the most appropriate region according to the accessibility requirements.
func pickRegion(accessibilityRequirements *csi.TopologyRequirement) (scw.Region, error) {
	if accessibilityRequirements == nil {
		return "", nil
	}

	requestedRegions := map[string]scw.Region{}
	for _, req := range accessibilityRequirements.GetRequisite() {
		topologyKeys := req.GetSegments()
		for topologyKey, topologyValue := range topologyKeys {
			switch topologyKey {
			case RegionTopologyKey:
				region, err := scw.ParseRegion(topologyValue)
				if err != nil {
					klog.Warningf("the given value for requisite %s: %s is not a valid region", RegionTopologyKey, topologyValue)
					continue
				}

				requestedRegions[topologyValue] = region
			default:
				klog.Warningf("unknown topology key %s for requisite", topologyKey)
			}
		}
	}

	preferredRegions := []scw.Region{}
	preferredRegionsMap := map[string]scw.Region{}
	for _, pref := range accessibilityRequirements.GetPreferred() {
		topologyKeys := pref.GetSegments()
		for topologyKey, topologyValue := range topologyKeys {
			switch topologyKey {
			case RegionTopologyKey:
				region, err := scw.ParseRegion(topologyValue)
				if err != nil {
					klog.Warningf("the given value for requisite %s: %s is not a valid region", RegionTopologyKey, topologyValue)
					continue
				}
				if _, ok := preferredRegionsMap[topologyValue]; !ok {
					if accessibilityRequirements.GetRequisite() != nil {
						if _, ok := requestedRegions[topologyValue]; !ok {
							return "", status.Errorf(codes.InvalidArgument, "%s: %s is specified in preferred but not in requisite", topologyKey, topologyValue)
						}
						delete(requestedRegions, topologyValue)
					}

					preferredRegionsMap[topologyValue] = region
					preferredRegions = append(preferredRegions, region)
				}
			default:
				klog.Warningf("unknow topology key %s for preferred", topologyKey)
			}
		}
	}

	for _, requestedRegion := range requestedRegions {
		preferredRegions = append(preferredRegions, requestedRegion)
	}

	if len(preferredRegions) == 0 {
		return "", errors.New("topology requirement is empty")
	}

	return preferredRegions[0], nil
}

func csiVolume(fs *file.FileSystem) *csi.Volume {
	return &csi.Volume{
		CapacityBytes: int64(fs.Size),
		VolumeId:      volumeID(fs.Region, fs.ID),
		AccessibleTopology: []*csi.Topology{
			{
				Segments: map[string]string{
					RegionTopologyKey: fs.Region.String(),
				},
			},
		},
	}
}

func volumeID(region scw.Region, fsID string) string {
	return fmt.Sprintf("%s/%s", region, fsID)
}

func extractIDAndRegion(volumeID string) (string, scw.Region, error) {
	if volumeID == "" {
		return "", "", errors.New("volumeID cannot be empty")
	}

	parts := strings.Split(volumeID, "/")
	if len(parts) > 2 {
		return "", "", status.Errorf(codes.InvalidArgument, "ID %q is not correctly formatted", volumeID)
	} else if len(parts) == 1 {
		return parts[0], "", nil
	} else { // id like region/uuid
		region, err := scw.ParseRegion(parts[0])
		if err != nil {
			klog.Warningf("wrong region in ID %q, will try default region", volumeID)
			return parts[1], "", nil //nolint:nilerr
		}
		return parts[1], region, nil
	}
}

func extractIDAndZone(serverID string) (string, scw.Zone, error) {
	if serverID == "" {
		return "", "", errors.New("serverID cannot be empty")
	}

	parts := strings.Split(serverID, "/")
	if len(parts) > 2 {
		return "", "", status.Errorf(codes.InvalidArgument, "ID %q is not correctly formatted", serverID)
	} else if len(parts) == 1 {
		return parts[0], "", nil
	} else { // id like zone/uuid
		zone, err := scw.ParseZone(parts[0])
		if err != nil {
			klog.Warningf("wrong zone in ID %q, will try default zone", serverID)
			return parts[1], "", nil //nolint:nilerr
		}
		return parts[1], zone, nil
	}
}

// validateVolumeCapabilities makes sure the provided volume capabilities are
// valid and supported by the driver. If optional is false and no volumeCapabilities
// are provided, an error is returned.
func validateVolumeCapabilities(volumeCapabilities []*csi.VolumeCapability, optional bool) error {
	if !optional && len(volumeCapabilities) == 0 {
		return errors.New("no volumeCapabilities were provided")
	}

	for i, volumeCapability := range volumeCapabilities {
		if err := validateVolumeCapability(volumeCapability); err != nil {
			return fmt.Errorf("unsupported volume capability at index %d: %w", i, err)
		}
	}

	return nil
}

// validateVolumeCapability validates a single volume capacity.
func validateVolumeCapability(volumeCapability *csi.VolumeCapability) (err error) {
	mode := volumeCapability.GetAccessMode().GetMode()
	if !slices.Contains(supportedAccessModes, mode) {
		return fmt.Errorf("mode %q not supported", mode.String())
	}

	if volumeCapability.GetMount() == nil {
		return errors.New("only mount volume type is supported")
	}

	return nil
}

// codeFromError takes an error and returns the most appropriate GRPC error code
// according to the CSI spec.
func codeFromScalewayError(err error) codes.Code {
	switch {
	case scaleway.IsInvalidArgumentsError(err):
		return codes.InvalidArgument
	case scaleway.IsNotFoundError(err):
		return codes.NotFound
	default:
		return codes.Internal
	}
}
