# 389 Directory Server operator

A Kubernetes operator for managing [389 Directory Server](https://www.port389.org/) instances.
It automates deployment, configuration, and lifecycle management of 389DS via a `DirectoryService` custom resource.

## Description

The ds-operator provides a declarative way to run 389 Directory Server on Kubernetes.
Define a `DirectoryService` CR and the operator handles:

- **StatefulSet management** -- ordered pod startup with stable network identities
- **Persistent storage** -- automatic PVC provisioning for the `/data` volume
- **Suffix (database backend) creation** -- configure LDAP suffixes declaratively
- **Directory Manager credentials** -- auto-generated or user-provided via Secret reference
- **Service exposure** -- headless Service for pod DNS and ClusterIP Service for LDAP/LDAPS

### Example CR

```yaml
apiVersion: dirsrv.operator.port389.org/v1alpha1
kind: DirectoryService
metadata:
  name: my-ds
spec:
  image: quay.io/389ds/dirsrv:latest
  replicas: 2
  suffixes:
    - name: userroot
      dn: "dc=example,dc=com"
  storage:
    size: "10Gi"
  ports:
    ldap: 3389
    ldaps: 3636
```

## Getting Started

### Prerequisites

- Go 1.24+
- Docker or Podman
- kubectl v1.28+
- Access to a Kubernetes v1.28+ cluster

### Development Setup

```sh
# Clone and set up pre-commit hooks
git clone https://github.com/389ds/ds-operator.git
cd ds-operator
make setup-hooks
```

### To Deploy on the cluster

**Build and push the operator image:**

```sh
make docker-build docker-push IMG=ghcr.io/389ds/ds-operator:v0.0.1
```

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the operator to the cluster:**

```sh
make deploy IMG=ghcr.io/389ds/ds-operator:v0.0.1
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create a DirectoryService instance:**

```sh
kubectl apply -f - <<EOF
apiVersion: dirsrv.operator.port389.org/v1alpha1
kind: DirectoryService
metadata:
  name: example-ds
spec:
  image: quay.io/389ds/dirsrv:3.1
  replicas: 1
  suffixes:
    - name: userroot
      dn: "dc=example,dc=com"
EOF
```

**Check the status:**

```sh
kubectl get dirsrv
```

### To Uninstall

**Delete the DirectoryService instances:**

```sh
kubectl delete directoryservices --all
```

**Delete the CRDs from the cluster:**

```sh
make uninstall
```

**Undeploy the operator from the cluster:**

```sh
make undeploy
```

## Testing

```sh
# Unit tests (with envtest)
make test

# Linting
make lint

# End-to-end tests (requires Kind; use Podman with KIND_EXPERIMENTAL_PROVIDER=podman)
make test-e2e

# With Podman
CONTAINER_TOOL=podman KIND_EXPERIMENTAL_PROVIDER=podman make test-e2e
```

## Project Distribution

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=ghcr.io/389ds/ds-operator:v0.0.1
```

This generates `dist/install.yaml` containing all resources needed to deploy the operator.

2. Install using the generated manifest:

```sh
kubectl apply -f https://raw.githubusercontent.com/389ds/ds-operator/main/dist/install.yaml
```

## Contributing

1. Fork the repository
2. Set up pre-commit hooks: `make setup-hooks`
3. Create a feature branch
4. Make changes and ensure all checks pass: `make test lint`
5. Submit a pull request

Run `make help` for all available targets.

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html).

## License

Copyright 2026 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
