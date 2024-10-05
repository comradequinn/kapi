package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/comradequinn/kapi"
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
