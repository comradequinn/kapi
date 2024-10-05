package kapi

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type (
	// Client can be used to perform various IO operations against resources on a k8s cluster
	Client[TItem client.Object, TList client.ObjectList] struct {
		getClient        func() (client.Client, error)
		resourceType     reflect.Type
		resourceListType reflect.Type
	}
	// Subresource represents a section of a resource that can be modified independently of the resource as a whole
	Subresource string
)

const (
	SubresourceStatus Subresource = "status"
	SubresourceScale  Subresource = "scale"
)

// Create creates a resource on the k8s cluster
func (c *Client[TItem, TList]) Create(ctx context.Context, resource TItem) error {
	defer c.observe(ctx, "create", resource)()

	clt, err := c.getClient()

	if err != nil {
		return err
	}

	return clt.Create(ctx, resource)
}

// Update modifies a resource on the k8s cluster.
// Optionally, specific subresources can be provided, which will limit updates to only those subresources
func (c *Client[TItem, TList]) Update(ctx context.Context, resource TItem, subresources ...Subresource) error {
	defer c.observe(ctx, "update", resource)()
	clt, err := c.getClient()

	if err != nil {
		return err
	}

	if len(subresources) == 0 {
		return clt.Update(ctx, resource)
	}

	for _, subresource := range subresources {
		if err = clt.SubResource(string(subresource)).Update(ctx, resource); err != nil {
			return fmt.Errorf("unable to update subresource %v. %v", subresource, err)
		}
	}

	return nil
}

// Delete removes a resource from the k8s cluster
func (c *Client[TItem, TList]) Delete(ctx context.Context, resource TItem) error {
	defer c.observe(ctx, "delete", resource)()

	clt, err := c.getClient()

	if err != nil {
		return err
	}

	return clt.Delete(ctx, resource)
}

// Get returns data describing the specified resource
func (c *Client[TItem, TList]) Get(ctx context.Context, namespace, name string) (TItem, error) {
	resource := reflect.New(c.resourceType).Interface().(TItem)

	defer c.observe(ctx, "get", resource)()

	clt, err := c.getClient()

	if err != nil {
		return resource, err
	}

	return resource, clt.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, resource)
}

// List returns data describing all occurences of the resource type associated with the client
func (c *Client[TItem, TList]) List(ctx context.Context) (TList, error) {
	resourceList := reflect.New(c.resourceListType).Interface().(TList)

	defer c.observe(ctx, "list", resourceList)()

	clt, err := c.getClient()

	if err != nil {
		return resourceList, err
	}

	return resourceList, clt.List(ctx, resourceList)
}

// ClientFor returns a Client that can be used to perform various IO operations against resources on a k8s cluster
//
// If cache is true, the client will use the cache to store and retrieve resources. This is more efficient and should
// be used by default.
//
// However, as there can be a delay before the latest resource state is available in the cache, some clients may need to
// disable it in order to retrieve the latest resource state from the cluster.
func ClientFor[TItem client.Object, TList client.ObjectList](ctx context.Context, cluster *Cluster, cache bool) *Client[TItem, TList] {
	var (
		zeroOfTItem TItem
		zeroOfTList TList
	)

	obs.LogFunc(ctx, 3, "creating kapi.client", "resource_type", fmt.Sprintf("%T", zeroOfTItem), "resource_list_type", fmt.Sprintf("%T", zeroOfTList))

	return &Client[TItem, TList]{
		getClient: func() (client.Client, error) {
			if !cluster.connected {
				panic("kapi.client used before kapi.cluster.connect called")
			}
			if !cache {
				return client.New(cluster.manager.GetConfig(), client.Options{
					Scheme: cluster.manager.GetScheme(),
				})
			}
			return cluster.manager.GetClient(), nil
		},
		resourceType:     reflect.TypeOf(zeroOfTItem).Elem(),
		resourceListType: reflect.TypeOf(zeroOfTList).Elem(),
	}
}

func (c *Client[TItem, TList]) observe(ctx context.Context, act string, obj runtime.Object) func() {
	stopTimer := obs.MetricTimerFunc(ctx, "kapi_client")

	var (
		zeroOfTItem TItem
		zeroOfTList TList
	)

	obs.LogFunc(ctx, 1, "kapi.client invoked", "type", "kapi_client_summary", "resource_action", act, "resource_type", fmt.Sprintf("%T", zeroOfTItem), "resource_list_type", fmt.Sprintf("%T", zeroOfTList))

	return func() {
		obs.LogFunc(ctx, 3, "kapi.client invoked", "type", "kapi_client_trace", "resource_action", act, "resource_type", fmt.Sprintf("%T", zeroOfTItem), "resource_list_type", fmt.Sprintf("%T", zeroOfTList), "resource", fmt.Sprintf("+%v", obj))
		stopTimer("resource_type", fmt.Sprintf("%T", zeroOfTItem), "resource_action", act)
	}
}
