# kapi

<!-- TOC -->

`kapi` provides a simplified interface to the [`controller-runtime`](https://github.com/kubernetes-sigs/controller-runtime) library.

It significantly reduces the amount of boilerplate, code-gen and complexity required to build Kubernetes controllers, operators and define CRDs.

## Features

- **Client Operations**: Perform CRUD operations on Kubernetes resources using a generic client interface.
- **Custom Resource Support**: Define and manage custom resources using generic types rather than code-gen.
- **Reconciliation**: Add custom reconciliation logic for Kubernetes resources with support for event filtering.
- **Observability**: Integrate structured logging and metrics for better observability of operations.
- **Validators**: Implement validation logic to ensure resources meet specific criteria before operations.

## Quick Start

For a fuller example of how to use various `kapi` features, see the [example](./cmd/example/) and consult the remainder of this README. 

The snippet below shows how to define a basic reconciler for a custom resource. Some boiler plate code related to imports and error handling is omitted for brevity.

```go
package main

import (
    // ... omitted for brevity ...
)

// Define a custom resource and its list form. Type aliases can be used to improve readability and reduce repetition of generic arguments.
type (
    ExampleResourceSpec struct {
        ExampleData string `json:"exampleData"`
    }
    // the example resource type only defines the spec, and omits status and scale using kapi.FieldUndefined
    ExampleResource     = kapi.CustomResource[ExampleResourceSpec, kapi.FieldUndefined, kapi.FieldUndefined]
    ExampleResourceList = kapi.CustomResourceList[*ExampleResource]
)

func main() {
    log, ctx := slog.New(slog.NewJSONHandler(os.Stdout, nil)), context.Background()

    // Initialize the kapi library with the logger to configure observability
    kapi.Init(kapi.UseSlog(ctx, log))

    // Create a new cluster to encapsulate the kubernetes context, by defining the namespace scope and the CRDs
    cluster, _ := kapi.NewCluster(ctx, kapi.ClusterConfig{
        Namespaces: []string{"kapi-quickstart"},
        CRDs: []kapi.CRDs{
            {
                APIGroup:   "kapi-quickstart.comradequinn.github.io",
                APIVersion: "v1",
                Kinds: map[string]kapi.KindType{
                    "ExampleResource":     &ExampleResource{},
                    "ExampleResourceList": &ExampleResourceList{},
                },
            },
        },
    })

    // Add a reconciler to the cluster to handle changes to the ExampleResource custom resource type
    kapi.AddReconciler(ctx, cluster, nil, func(ctx context.Context, evt kapi.ReconcileEventType, exampleResource *ExampleResource) error {

        // Create a client for the ExampleResource custom resource
        klient := kapi.ClientFor[*ExampleResource, *ExampleResourceList](ctx, cluster, true)

        // List all ExampleResource custom resources in the cluster
        exampleResources, _ := klient.List(ctx)

        // Log the number of ExampleResource custom resources in the cluster
        log.Info("example resource reconciled", "count", len(exampleResources.Items))

        return nil
    })

    // Connect to the cluster to begin processing events in the reconciler
    if err := cluster.Connect(ctx); err != nil {
        log.Error("failed to connect to cluster", "error", err)
    }
}
```

To spin up a local k8s cluster and deploy the full [example](./cmd/example/), run `make example`.

### Disclaimer

`kapi` is not a replacement for `controller-runtime`. 

It is a higher level library that sits on top of `controller-runtime` and provides a simplified and opinionated approach to building controllers, operators and defining CRDs. As such it may not be a good fit for all use cases. 

The main trade-off is its purely library orientated approach. This is in contrast to the code-gen based approach offered by kubebuilder; which is often used with `controller-runtime`. This approach, however, leaves manifest generation to other tools, though these are arguably better suited to that task for many use-cases (*for example, AI-powered code editors such as Cursor or Co-Pilot, that can readily generate YAML directly from go struct definitions*).

## Installation

To install `kapi`, use `go get`:

```bash
go get github.com/comradequinn/kapi
```

## Usage

### Initialising Observability

Configure observability for all `kapi` clusters using the `Init` function. 

In the following example, `UseSlog` is used to set up basic structured logging and metrics output using the `slog` package.

```go
import (
    // ... omitted for brevity ...
)

func main() {
    ctx := context.Background()

    obs := kapi.UseSlog(ctx, slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))

    kapi.Init(obs)

    // use kapi...
}
```

In this alternative example, custom implementations for `LogFunc` and `MetricTimerFunc` are provided to integrate with other logging and metrics providers. 

```go
kapi.Init(kapi.ObservabilityConfig{
    BackgroundContext: context.Background(),
    NewCorrelationCtx: yourNewCorrelationCtxFunc,
    LogFunc:           yourLogFunc,
    MetricTimerFunc:   yourMetricTimerFunc,
})
```

### Creating a Cluster

Create a new `kapi.Cluster` to encapsulate the kubernetes context, by defining the namespace scope and the CRDs. If you are implementing any [hooks](#adding-hooks), you will need to provide the TLS certificate location too.

```go
cluster, _ := kapi.NewCluster(ctx, kapi.ClusterConfig{
    Namespaces: []string{"kapi-quickstart"},
    CRDs: []kapi.CRDs{
        {
            APIGroup:   "kapi.comradequinn.github.io",
            APIVersion: "v1",
            Kinds: map[string]kapi.KindType{
                "ExampleResource":     &ExampleResource{},
                "ExampleResourceList": &ExampleResourceList{},
            },
        },
    },
})
```

### Adding Hooks

Hooks provide admission control functionality, allowing you to validate or apply defaults values to resources before CRUD operations occur. These are typically used for enforcing business rules or setting default values.

The `Hook` type supports several operations:
- `ValidateCreateFunc`: Validates resources before creation
- `ValidateUpdateFunc`: Validates resources before updates
- `ValidateDeleteFunc`: Validates resources before deletion
- `DefaulterFunc`: Sets default values for new resources

Any of these functions can be omitted if not required and will default to a validation success or a no-op defaulter.

In the example below, a `ValidateCreateFunc` is provided to ensure the `ExampleData` field is set:

```go
import (
    // ... omitted for brevity ...
)

func main() {
    // ... kapi initialisation code omitted for brevity ...

    err := kapi.AddHook(ctx, cluster, &kapi.Hook[*ExampleResource]{
        ValidateCreateFunc: func(ctx context.Context, resource *ExampleResource) (warnings []string, err error) {
            if resource.Spec.ExampleData == "" {
                return nil, fmt.Errorf("example-data is required")
            }
            return nil, nil
        },
        // other validation functions and/or a defaulting function can also be provided...
    })
}
```

### Adding a Reconciler

Add a reconciler to handle resource events for a specific resource type. The resource type itself is inferred from the argument passed to the `reconcilerFunc` parameter. 

In this example, a filter is also applied to respond only to create events:

```go
import (
    // ... omitted for brevity ...
)

func main() {
    // ... kapi initialisation code omitted for brevity ...

    reconcileFilterFunc := func(e kapi.ResourceEventType, o client.Object) bool {
        return e == kapi.ResourceEventCreate
    }

    err := kapi.AddReconciler(context.Background(), cluster, reconcileFilterFunc, func(ctx context.Context, eventType kapi.ReconcileEventType, resource *ExampleResource) error {
        // optionally create a client for the resource type
        klient := kapi.ClientFor[*ExampleResource, *ExampleResourceList](ctx, cluster, true)

        // perform operations using the client and another reconciler logic...

        // return nil to indicate success, an error will trigger a requeue
        return nil
    })
}
```

### Connecting to the Cluster

Connect to the cluster to start all configured reconcilers and enable the client cache:

```go
err = cluster.Connect(context.Background())
if err != nil {
    log.Fatalf("Failed to connect to cluster: %v", err)
}
```

### Using the Client

The `kapi.Client` provides a convenient way to perform various I/O operations against resources on a Kubernetes cluster. It supports operations such as creating, updating, deleting, getting, and listing resources.

### Creating a Client

To create a client for a specific resource type, use the `ClientFor` function. This function requires a context, a cluster, and a boolean indicating whether to use caching.

```go
klient := kapi.ClientFor[*ExampleResource, *ExampleResourceList](ctx, cluster, true)
```

Caching should typically be enabled as it is more efficient. However, there can be a delay before the latest resource state is available in the cache. If your application requires the most up-to-date resource state immediately, you may need to disable caching.

#### Client Operations

Once you have a client, you can perform various operations:

##### Create a Resource

Use the `Create` method to add a new resource to the cluster.

```go
exampleResource := &ExampleResource{
    Spec: ExampleResourceSpec{
        ExampleData: "initial value",
    },
}
exampleResource.Name = "example-name"
exampleResource.Namespace = "example-namespace"

err := klient.Create(ctx, exampleResource)
```
##### Get a Resource

Retrieve a specific resource using the `Get` method.

```go
resource, err := klient.Get(ctx, "example-namespace", "example-name")
```

##### List Resources

List all resources of a specific type with the `List` method.

```go
resources, err := klient.List(ctx)
```

##### Update a Resource

Modify an existing resource using the `Update` method.

```go
// ... code to get the resource to update omitted for brevity ...

exampleResource.Spec.ExampleData = "updated value"

err = klient.Update(ctx, exampleResource)

```

In some cases, only subresource(s) may require updating, in which case the subresource(s) can be specified with variadic argument, as shown below.


```go
// ... code to get the resource to update omitted for brevity ...

exampleResource.Status.Active = true // modifiy the subresource as required

err = klient.Update(ctx, exampleResource, "status") // specify that the update applies only to the subresource

```

##### Delete a Resource

Remove a resource from the cluster with the `Delete` method.

```go
// ... code to get the resource to update omitted for brevity ...

err = klient.Delete(ctx, exampleResource)

```

### Defining Custom Resources

Define custom resources using the `CustomResource` and `CustomResourceList` structs. An example is shown below:

```go
import "github.com/comradequinn/kapi"

type (
    ExampleResource     = kapi.CustomResource[ExampleResourceSpec, kapi.FieldUndefined, kapi.FieldUndefined]
    ExampleResourceList = kapi.CustomResourceList[*ExampleResource]

    ExampleResourceSpec struct {
        ExampleData string `json:"exampleData"`
    }
)
```

In this example, `kapi.FieldUndefined` is used as a placeholder for fields that are not needed in the custom resource definition. This allows you to focus on defining only the necessary fields, such as `Spec`, while omitting others like `Status` or `Scale` if they are not required.

Using type aliases, like `ExampleResource` and `ExampleResourceList` in the snippet above, improves code clarity both by providing meaningful names for types and by reducing the repetition of generic type arguments.

### Deployment

The lib-oriented approach of `kapi` allows for the definition and deployment of controllers and operators in a way that better suits existing architectures and deployment pipelines.

Deploying a `kapi`-based controller is simply a matter of creating a `deployment` in your usual manner. By default a single replica deployment model is inferred by `kapi`, however multiple replicas may be configured to enable high availabilty, in which case the `LeaderElection` field on the `kapi.ClusterConfig`. As shown below.

```go
cluster, _ := kapi.NewCluster(ctx, kapi.ClusterConfig{
    Namespaces: []string{"kapi-quickstart"},
    LeaderElection: kapi.LeaderElectionConfig{
        Enabled:      true,
        LockResource: "kapi-quickstart-leader-election-lock",
    },
})
```

## Metrics and Logging

The `kapi` package provides comprehensive observability through structured logging and metrics. Here's an overview of the types of metrics and logs emitted:

### Logging

- **Log Levels**: 
  - **Level 0**: Error logs, indicating critical issues that need immediate attention.
  - **Level 1**: Warning logs, highlighting potential issues or important events.
  - **Level 2**: Info logs, providing general information about the application's operation.
  - **Level 3**: Debug logs, offering detailed insights for troubleshooting and development.

- **Log Messages**:
  - **Client Operations**: Logs are emitted for each CRUD operation (`create`, `update`, `delete`, `get`, `list`) performed by the `Client` with details about the resource type and action. These can be identified with `type=kapi_client_summary` or `type=kapi_client_trace`; with the latter also containing additional trace information.
  - **Hook Events**: Logs are generated when hooks are triggered, including validation results and any defaults applied. These can be identified with `type=kapi_hook_summary` or `type=kapi_hook_trace`; with the latter also containing additional trace information.
  - **Reconciler Events**: Logs are generated when a reconciler is invoked, including the resource name, type, and event type (created, updated, deleted). These can be identified with `type=kapi_reconciler_summary` or `type=kapi_reconciler_trace`; with the latter also containing additional trace information.
  - **Cluster Operations**: Logs are produced during cluster creation and connection, detailing the namespaces and CRDs involved.

### Metrics

- **Metric Timers**: 
  - **`kapi_client`**: Measures the duration of client operations, providing insights into the performance of CRUD actions.
  - **`kapi_hook`**: Tracks the execution time of hook operations, including validation and default value application.
  - **`kapi_new_cluster`**: Tracks the time taken to create a new cluster, helping identify potential bottlenecks in cluster initialization.
  - **`kapi_connect`**: Monitors the time required to connect to a cluster, ensuring efficient startup of reconcilers.
  - **`kapi_add_reconciler`**: Captures the time spent adding a reconciler, useful for understanding the setup overhead.
  - **`kapi_reconcile`**: Records the duration of reconciliation processes, aiding in performance analysis of resource event handling.

### Correlation

Each log entry includes a `correlation_id` to trace and correlate events across different components and operations, enhancing the ability to diagnose issues and understand system behaviour.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- Built on top of the [`controller-runtime`](https://github.com/kubernetes-sigs/controller-runtime) library.