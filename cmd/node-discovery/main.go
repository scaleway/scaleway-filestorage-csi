package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/scaleway/scaleway-filestorage-csi/pkg/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer stop()

	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		klog.Fatal("NODE_NAME environment variable must be set")
	}

	fileStorageCompatibility, err := discovery.HasFileStorageCompatibility(ctx)
	if err != nil {
		klog.Fatalf("Failed to check if node has FileStorage compatibility: %s", err)
	}

	clientset, err := newKubernetesClient()
	if err != nil {
		klog.Fatal(err)
	}

	if err := discovery.UpdateFileStorageCompatibilityNodeLabel(ctx, clientset, nodeName, fileStorageCompatibility); err != nil {
		klog.Fatalf("Failed to update FileStorage compatibility node label: %s", err)
	}

	klog.Info("Waiting forever")
	<-ctx.Done()
}

func newKubernetesClient() (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from flags: %w", err)
		}
	} else {
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to create in-cluster Kubernetes config: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return clientset, nil
}
