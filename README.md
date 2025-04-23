# Scaleway File Storage CSI driver

## Features

Here is a list of features implemented by the Scaleway File Storage CSI driver.

### File Systems resizing

The Scaleway File Storage CSI driver implements the resize feature ([example for Kubernetes](https://kubernetes.io/blog/2018/07/12/resizing-persistent-volumes-using-kubernetes/)).
It allows an online resize (without the need to detach the File System).
However resizing can only be done upwards, decreasing a volume's size is not supported.

### ReadWriteMany (RWX) Volume

ReadWriteMany Volumes can be mounted as read-write by many nodes.
To create a RWX Volume, `ReadWriteMany` needs to be added to the `accessModes`.
For instance, here is a PVC with `ReadWriteMany` enabled:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: my-rwx-pvc
spec:
  accessModes:
    - ReadWriteMany
  [...]
```

## Kubernetes

This section is Kubernetes specific. Note that Scaleway CSI driver may work for
older Kubernetes versions than those announced. The CSI driver allows to use
[Persistent Volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)
in Kubernetes.

### Kubernetes Version Compatibility Matrix

| Scaleway CSI Driver \ Kubernetes Version | Min K8s Version | Max K8s Version |
| ---------------------------------------- | --------------- | --------------- |
| master branch                            | v1.20           | -               |

### Examples

Some examples are available [here](./examples/kubernetes).

### Installation

These steps will cover how to install the Scaleway File Storage CSI driver in your
Kubernetes cluster, using Helm.

#### Requirements

- A Kubernetes cluster running on Scaleway instances (v1.20+)
- Scaleway Project or Organization ID, Access and Secret key
- Helm v3

#### Deployment

1. Add the Scaleway Helm repository.

    ```bash
    helm repo add scaleway https://helm.scw.cloud/
    helm repo update
    ```

2. Deploy the latest release of the `scaleway-filestorage-csi` Helm chart.

    ```bash
    helm upgrade --install scaleway-filestorage-csi --namespace kube-system scaleway/scaleway-filestorage-csi \
        --set controller.scaleway.env.SCW_DEFAULT_REGION=fr-par \
        --set controller.scaleway.env.SCW_DEFAULT_ZONE=fr-par-1 \
        --set controller.scaleway.env.SCW_DEFAULT_PROJECT_ID=11111111-1111-1111-1111-111111111111 \
        --set controller.scaleway.env.SCW_ACCESS_KEY=ABCDEFGHIJKLMNOPQRST \
        --set controller.scaleway.env.SCW_SECRET_KEY=11111111-1111-1111-1111-111111111111
    ```

    Review the [configuration values](https://github.com/scaleway/helm-charts/blob/master/charts/scaleway-filestorage-csi/values.yaml)
    for the Helm chart.

3. You can now verify that the driver is running:

    ```bash
    $ kubectl get pods -n kube-system
    [...]
    scaleway-filestorage-csi-controller-76897b577d-b4dgw   8/8     Running   0          3m
    scaleway-filestorage-csi-node-hvkfw                    3/3     Running   0          3m
    scaleway-filestorage-csi-node-jmrz2                    3/3     Running   0          3m
    [...]
    ```

    You should see the scaleway-filestorage-csi-controller and the scaleway-filestorage-csi-node pods.

## Development

### Build

You can build the Scaleway File Storage CSI driver executable using the following commands:

```bash
make compile
```

You can build a local docker image named scaleway-filestorage-csi for your current architecture
using the following command:

```bash
make docker-build
```

### Test

In order to run the tests:

```bash
make test
```

In addition to unit tests, we provide tools to run the following tests:

- [Kubernetes external storage e2e tests](./test/e2e/)
- [CSI sanity tests](./test/sanity/)

### Contribute

If you are looking for a way to contribute please read the [contributing guide](./CONTRIBUTING.md)

### Code of conduct

Participation in the Kubernetes community is governed by the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/master/code-of-conduct.md).

## Reach us

We love feedback. Feel free to reach us on [Scaleway Slack community](https://slack.scaleway.com),
we are waiting for you on #k8s.

You can also join the official Kubernetes slack on #scaleway-k8s channel

You can also [raise an issue](https://github.com/scaleway/scaleway-filestorage-csi/issues/new)
if you think you've found a bug.
