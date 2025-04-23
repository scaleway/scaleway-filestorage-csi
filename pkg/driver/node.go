package driver

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/scaleway/scaleway-filestorage-csi/pkg/scaleway"
	"github.com/scaleway/scaleway-sdk-go/scw"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
)

type nodeService struct {
	csi.UnimplementedNodeServer

	mounter mount.Interface

	nodeID            string
	nodeZone          scw.Zone
	nodeRegion        scw.Region
	maxVolumesPerNode int64
}

func newNodeService() (*nodeService, error) {
	metadata, err := scaleway.GetMetadata()
	if err != nil {
		return nil, fmt.Errorf("unable to fetch Scaleway metadata: %w", err)
	}

	zone, err := scw.ParseZone(metadata.Location.ZoneID)
	if err != nil {
		return nil, fmt.Errorf("invalid zone in metadata: %w", err)
	}

	region, err := zone.Region()
	if err != nil {
		return nil, fmt.Errorf("unable to extract region from zone %q: %w", zone, err)
	}

	client, err := scaleway.NewUnauthenticatedClient(UserAgent())
	if err != nil {
		return nil, err //nolint:wrapcheck
	}

	serverType, err := client.GetServerType(context.TODO(), metadata.CommercialType, zone)
	if err != nil {
		return nil, err
	}

	if serverType.Capabilities == nil || serverType.Capabilities.MaxFileSystems == 0 {
		return nil, fmt.Errorf("this node with type %q is not compatible with this CSI", metadata.CommercialType)
	}

	return &nodeService{
		nodeID:            metadata.ID,
		nodeZone:          zone,
		nodeRegion:        region,
		mounter:           &mount.Mounter{},
		maxVolumesPerNode: int64(serverType.Capabilities.MaxFileSystems),
	}, nil
}

// NodeStageVolume is called by the CO prior to the volume being consumed
// by any workloads on the node by NodePublishVolume.
// The Plugin SHALL assume that this RPC will be executed on the node
// where the volume will be used.
// This RPC SHOULD be called by the CO when a workload that wants to use
// the specified volume is placed (scheduled) on the specified node
// for the first time or for the first time since a NodeUnstageVolume call
// for the specified volume was called and returned success on that node.
func (d *nodeService) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	klog.V(4).Infof("NodeStageVolume called with request %+v", req)

	id, _, err := extractIDAndRegion(req.GetVolumeId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	stagingTargetPath := req.GetStagingTargetPath()
	if stagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "stagingTargetPath not provided")
	}

	volumeCapability := req.GetVolumeCapability()
	if volumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volumeCapability not provided")
	}

	if err := validateVolumeCapability(volumeCapability); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "volumeCapability not supported: %s", err)
	}

	notMountPoint, err := d.mounter.IsLikelyNotMountPoint(stagingTargetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(stagingTargetPath, 0750); err != nil {
				return nil, status.Errorf(codes.Internal, "failed to mkdir staging target path: %s", err)
			}
		} else {
			return nil, status.Errorf(codes.Internal, "unable to determine if target path is mount point: %s", err)
		}
	}

	if !notMountPoint {
		// Already mounted.
		return &csi.NodeStageVolumeResponse{}, nil
	}

	mountCap := volumeCapability.GetMount()
	if mountCap == nil {
		return nil, status.Error(codes.InvalidArgument, "mount volume capability is nil")
	}

	if err := d.mounter.Mount(id, stagingTargetPath, "virtiofs", mountCap.GetMountFlags()); err != nil {
		return nil, status.Errorf(codes.Internal, "unable to mount volume: %s", err)
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

// NodeUnstageVolume is a reverse operation of NodeStageVolume.
// It must undo the work by the corresponding NodeStageVolume.
func (d *nodeService) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	klog.V(4).Infof("NodeUnstageVolume called with request %+v", req)

	stagingTargetPath := req.GetStagingTargetPath()
	if stagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "stagingTargetPath not provided")
	}

	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volumeID not provided")
	}

	if err := mount.CleanupMountPoint(stagingTargetPath, d.mounter, false); err != nil {
		return nil, status.Errorf(codes.Internal, "unable to unmount volume: %s", err)
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodePublishVolume is called by the CO when a workload
// that wants to use the specified volume is placed (scheduled)
// on a node. The Plugin SHALL assume that this RPC will be executed
// on the node where the volume will be used.
func (d *nodeService) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	klog.V(4).Infof("NodePublishVolume called with request %+v", req)

	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "targetPath not provided")
	}

	volumeCapability := req.GetVolumeCapability()
	if volumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "volumeCapability not provided")
	}

	stagingTargetPath := req.GetStagingTargetPath()
	if stagingTargetPath == "" {
		return nil, status.Error(codes.FailedPrecondition, "stagingTargetPath not provided")
	}

	if err := validateVolumeCapability(volumeCapability); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "volumeCapability not supported: %s", err)
	}

	notMountPoint, err := d.mounter.IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := os.MkdirAll(targetPath, 0750); err != nil {
				return nil, status.Errorf(codes.Internal, "failed to mkdir target path: %s", err)
			}
		} else {
			return nil, status.Errorf(codes.Internal, "unable to determine if target path is mount point: %s", err)
		}
	}

	if !notMountPoint {
		// Already mounted.
		return &csi.NodePublishVolumeResponse{}, nil
	}

	mountOptions := append([]string{"bind"}, req.GetVolumeCapability().GetMount().GetMountFlags()...)

	if req.GetReadonly() {
		mountOptions = append(mountOptions, "ro")
	}

	if err := d.mounter.Mount(stagingTargetPath, targetPath, "virtiofs", mountOptions); err != nil {
		return nil, status.Errorf(codes.Internal, "error mounting source %s to target %s: %s", stagingTargetPath, targetPath, err)
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume is a reverse operation of NodePublishVolume.
// This RPC MUST undo the work by the corresponding NodePublishVolume.
func (d *nodeService) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	klog.V(4).Infof("NodeUnpublishVolume called with request %+v", req)

	targetPath := req.GetTargetPath()
	if targetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "targetPath not provided")
	}

	if err := mount.CleanupMountPoint(targetPath, d.mounter, false); err != nil {
		return nil, status.Errorf(codes.Internal, "unable to unmount volume: %s", err)
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetCapabilities allows the CO to check the supported capabilities of node service provided by the Plugin.
func (d *nodeService) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	klog.V(4).Infof("NodeGetCapabilities called with request %+v", req)

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_SINGLE_NODE_MULTI_WRITER,
					},
				},
			},
		},
	}, nil
}

// NodeGetInfo returns information about node's volumes
func (d *nodeService) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	klog.V(4).Infof("NodeGetInfo called with request %+v", req)

	return &csi.NodeGetInfoResponse{
		NodeId:            d.nodeZone.String() + "/" + d.nodeID,
		MaxVolumesPerNode: d.maxVolumesPerNode,
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				RegionTopologyKey: d.nodeRegion.String(),
			},
		},
	}, nil
}
