[![gh-pages](https://github.com/STollenaar/cmstate-injector-operator/actions/workflows/pages/pages-build-deployment/badge.svg?branch=gh-pages)](https://github.com/STollenaar/cmstate-injector-operator/actions/workflows/pages/pages-build-deployment)
[![Helm Charts](https://github.com/STollenaar/cmstate-injector-operator/actions/workflows/release.yml/badge.svg)](https://github.com/STollenaar/cmstate-injector-operator/actions/workflows/release.yml)
[![Docker Builds](https://github.com/STollenaar/cmstate-injector-operator/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/STollenaar/cmstate-injector-operator/actions/workflows/docker-publish.yml)

# cmstate-injector-operator

## Overview

`cmstate-injector-operator` is a Kubernetes operator that simplifies the management of ConfigMaps by dynamically creating them based on the specifications defined in Custom Resource Definitions (CRDs). The operator focuses on the interaction between `CMTemplate` and `CMState` CRDs, offering a streamlined approach to ConfigMap management in a Kubernetes cluster.

## Features

- **CMTemplate CRD:** Define a template for ConfigMaps using the `CMTemplate` CRD. This includes specifying annotations for template replacement values.

- **CMState CRD:** Track the usage of ConfigMaps by Pods through the `CMState` CRD. Maintain an audience list to identify Pods utilizing the created ConfigMap.

- **Reconcile Loop:** The operator performs reconciliation for `CMState` and `CMTemplate` CRDs, ensuring that the ConfigMap state aligns with the desired specifications.

- **Mutating Webhook:** Triggered on Pod creation and deletion, the mutating webhook watches for a specific annotation, `cache.spices.dev/cmtemplate`, targeting a valid and created `CMTemplate`.

## Getting Started

### Prerequisites

- Kubernetes cluster
- `kubectl` configured to access the cluster

Only if modifying this operator by adding more controllers should you need this:

- [operator-sdk](https://github.com/operator-framework/operator-sdk) installed

### Installation

#### Operator Deployment

1. Clone the repository:

   ```bash
   git clone https://github.com/your-username/cmstate-injector-operator.git
   cd cmstate-injector-operator
   ```

2. Build and deploy the operator:

   ```bash
   make install
   make deploy
   ```

3. Verify the deployment:

   ```bash
   kubectl get pods -n <namespace>
   ```

#### Helm Chart Deployment

Alternatively, you can deploy the operator using the Helm chart located in the `charts` directory.

1. Change into the `charts` directory:

   ```bash
   cd charts
   ```

2. Install the Helm chart:

   ```bash
   helm install cmstate-injector-operator .
   ```

3. Verify the deployment:

   ```bash
   kubectl get pods -n <namespace>
   ```

## Usage

1. Define a `CMTemplate` to specify the template for your ConfigMap:

   ```yaml
    apiVersion: cache.spices.dev/v1alpha1
    kind: CMTemplate
    metadata:
        name: cmtemplate-example
    spec:
    template:
        annotationreplace:
            example-annotation: '{example-regex}'
        cmtemplate:
            config.ini: |
                name = {example-regex}
                write_to_example = true
            targetAnnotation: example-target-annotation
   ```

2. Create a `CMState` to track ConfigMap usage:

   ```yaml
    apiVersion: cache.spices.dev/v1alpha1
    kind: CMState
    metadata:
        name: cmstate-example
        namespace: example
    spec:
        audience:
        - kind: Pod
            name: example
        cmtemplate: cmtemplate-example
        target: cmstate-example-configmap
   ```

3. Pods that reference the created ConfigMap should include the annotation:

   ```yaml
   metadata:
     annotations:
       cache.spices.dev/cmtemplate: cmtemplate-example
   ```

## Contributing

Contributions are welcome! Please check out our [contribution guidelines](CONTRIBUTING.md) for more details.

## License

This project is licensed under the [MIT License](LICENSE). See the LICENSE file for details.

## Acknowledgments

- [operator-sdk](https://github.com/operator-framework/operator-sdk)
- Kubernetes community

## Contact

For any questions or feedback, please reach out to [stephen@tollenaar.com](mailto:stephen@tollenaar.com).
