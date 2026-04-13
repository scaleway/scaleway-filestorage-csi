# Kubernetes examples

You can find in this directory some examples about how to use the Scaleway File Storage CSI
driver inside Kubernetes.

It will cover [Persistent Volumes/Persistent Volume Claims (PV/PVC)](https://kubernetes.io/docs/concepts/storage/persistent-volumes/),
and [Storage Classes](https://kubernetes.io/docs/concepts/storage/storage-classes/).

If a [StorageClass](https://kubernetes.io/docs/concepts/storage/storage-classes/)
is not provided in the examples, the `sfs-standard` storage class will be used.

## PVC & Deployment

We will create a [PersistentVolumeClaim](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)
and use it as a volume inside the pod of a deployment, to store nginx's HTML directory.
First, we will create a 3Gi volume:

```bash
kubectl apply -f pvc-deployment/pvc.yaml
```

Now we can create the deployment that will use this volume:

```bash
kubectl apply -f pvc-deployment/deployment.yaml
```

## Importing existing Scaleway volumes

If you have an already existing File System, with the ID `11111111-1111-1111-111111111111`
in the region `fr-par`, you can import it by creating the following PV:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: test-pv
spec:
  capacity:
    storage: 100G
  volumeMode: Filesystem
  accessModes:
    - ReadWriteOnce
  storageClassName: sfs-standard
  csi:
    driver: filestorage.csi.scaleway.com
    volumeHandle: fr-par/11111111-1111-1111-111111111111
  nodeAffinity:
    required:
      nodeSelectorTerms:
      - matchExpressions:
        - key: topology.filestorage.csi.scaleway.com/region
          operator: In
          values:
          - fr-par
```

Once the PV is created, create a PVC with the same attributes (here `sfs-standard`
as storage class and a size of 100G):

```bash
kubectl apply -f importing/pvc.yaml
```

And finally create a pod that uses this volume:

```bash
kubectl apply -f importing/pod.yaml
```

## Different StorageClass

[StorageClasses](https://kubernetes.io/docs/concepts/storage/storage-classes/)
offer a way to easily create different types of [Volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/).

In the installation guide, a basic storage class is deployed: `sfs-standard` that
will provision standard Scaleway File Systems. We will see here how to customize
different storage classes. The provisioner will always be `filestorage.csi.scaleway.com`.

### Set a default storage class

In order to have a default storage class (i.e. not having to specify the `storageClassName`
for each PVC), you must add the `storageclass.kubernetes.io/is-default-class: "true"`
annotation to the storage class:

```yaml
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: my-default-storage-class
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: filestorage.csi.scaleway.com
reclaimPolicy: Delete
```

### Specify in which region the File Systems are going to be created

By default, the Scaleway File Storage CSI plugin uses the `SCW_DEFAULT_REGION` environment
variable to get the region where the volumes will be provisioned. If you want to
override this value, you must use the `allowedTopologies` field of the storage
class to specify a region:

```yaml
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: my-ams-storage-class
provisioner: filestorage.csi.scaleway.com
reclaimPolicy: Delete
allowedTopologies:
- matchLabelExpressions:
  - key: topology.filestorage.csi.scaleway.com/region
    values:
    - nl-ams
```
