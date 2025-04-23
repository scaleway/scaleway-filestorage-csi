package discovery

import (
	"context"
	"fmt"

	"github.com/scaleway/scaleway-filestorage-csi/pkg/driver"
	"github.com/scaleway/scaleway-filestorage-csi/pkg/scaleway"
	"github.com/scaleway/scaleway-sdk-go/scw"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

const (
	// FileStorageCompatibilityNodeLabel is the label added to nodes that are compatible
	// with the Scaleway File Storage product.
	FileStorageCompatibilityNodeLabel = driver.DriverName + "/is-compatible"

	TrueLabelValue = "true"
)

// HasFileStorageCompatibility returns true if the node where this function is running
// is compatible with the Scaleway File Storage product.
func HasFileStorageCompatibility(ctx context.Context) (bool, error) {
	metadata, err := scaleway.GetMetadata()
	if err != nil {
		return false, fmt.Errorf("unable to fetch Scaleway metadata: %w", err)
	}

	zone, err := scw.ParseZone(metadata.Location.ZoneID)
	if err != nil {
		return false, fmt.Errorf("invalid zone in metadata: %w", err)
	}

	klog.Infof("Node with type %s is located in %s", metadata.CommercialType, zone)

	client, err := scaleway.NewUnauthenticatedClient(driver.UserAgent())
	if err != nil {
		return false, err
	}

	serverType, err := client.GetServerType(ctx, metadata.CommercialType, zone)
	if err != nil {
		return false, err
	}

	if serverType.Capabilities == nil || serverType.Capabilities.MaxFileSystems == 0 {
		return false, nil
	}

	return true, nil
}

// UpdateFileStorageCompatibilityNodeLabel adds or removes the compatibility label
// on the specified Kubernetes Node depending on the "compatible" param provided.
func UpdateFileStorageCompatibilityNodeLabel(ctx context.Context, client kubernetes.Interface, nodeName string, compatible bool) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		node, err := client.CoreV1().Nodes().Get(ctx, nodeName, v1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get node %s: %w", nodeName, err)
		}

		if ensureFileStorageCompatibilityNodeLabel(node, compatible) {
			if _, err := client.CoreV1().Nodes().Update(ctx, node, v1.UpdateOptions{}); err != nil {
				return fmt.Errorf("failed to update node %s: %w", nodeName, err)
			}
		}

		return nil
	})
}

// ensureFileStorageCompatibilityNodeLabel adds or removes the compatibility label
// on the provided Node depending on the "compatible" param provided. It returns
// true if something was changed.
func ensureFileStorageCompatibilityNodeLabel(node *corev1.Node, compatible bool) bool {
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}

	if compatible {
		if node.Labels[FileStorageCompatibilityNodeLabel] != TrueLabelValue {
			node.Labels[FileStorageCompatibilityNodeLabel] = TrueLabelValue
			return true
		}
	} else {
		if _, ok := node.Labels[FileStorageCompatibilityNodeLabel]; ok {
			delete(node.Labels, FileStorageCompatibilityNodeLabel)
			return true
		}
	}

	return false
}
