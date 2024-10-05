package kapi

import (
	"context"
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Hook defines admission control functionality for validating and applying default values to resources before CRUD operations occur.
//
// A hook is typically used for enforcing business rules or setting default values by implementing one or more of the following functions:
// - ValidateCreateFunc: Validates resources before creation
// - ValidateUpdateFunc: Validates resources before updates
// - ValidateDeleteFunc: Validates resources before deletion
// - DefaulterFunc: Sets default values for new resources
// Any of these functions can be omitted (left as nil) if the desired behavior is not required.
type Hook[T client.Object] struct {
	DefaulterFunc      func(ctx context.Context, resource T) error
	ValidateCreateFunc func(ctx context.Context, resource T) (warnings []string, err error)
	ValidateUpdateFunc func(ctx context.Context, oldResource, newResource T) (warnings []string, err error)
	ValidateDeleteFunc func(ctx context.Context, resource T) (warnings []string, err error)
}

// AddHook registers a hook with the provided cluster.
//
// Hooks defines admission control functionality for validating and applying default values to resources before CRUD operations occur.
func AddHook[T client.Object](ctx context.Context, cluster *Cluster, hook *Hook[T]) error {
	if cluster.connected {
		panic("kapi.add-hook must be called before kapi.cluster.connect")
	}

	var zeroOfT T

	defer obs.MetricTimerFunc(ctx, "kapi_add_hook")("resource_type", fmt.Sprintf("%T", zeroOfT))
	obs.LogFunc(ctx, 3, "creating kapi.hook", "resource_type", fmt.Sprintf("%T", zeroOfT))

	t := reflect.New(reflect.TypeOf(zeroOfT).Elem()).Interface().(T)

	ctrl.NewWebhookManagedBy(cluster.manager).
		For(t).
		WithValidator(hook).
		WithDefaulter(hook).
		Complete()

	return nil
}

func (h *Hook[T]) Default(ctx context.Context, obj runtime.Object) error {
	if h.DefaulterFunc == nil {
		return nil
	}

	defer h.observe(ctx, "default", obj)()

	resource, ok := obj.(T)

	if !ok {
		return fmt.Errorf("defaulter for custom resource of %T was passed type of %T", h, obj)
	}

	return h.DefaulterFunc(ctx, resource)
}

func (h *Hook[T]) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	if h.ValidateCreateFunc == nil {
		return nil, nil
	}

	defer h.observe(ctx, "create", obj)()

	resource, ok := obj.(T)

	if !ok {
		return nil, fmt.Errorf("creation validator for custom resource of %T was passed type of %T", h, obj)
	}

	return h.ValidateCreateFunc(ctx, resource)
}

func (h *Hook[T]) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	if h.ValidateUpdateFunc == nil {
		return nil, nil
	}

	defer h.observe(ctx, "update", newObj)()

	newResource, okNew := newObj.(T)
	oldResource, okOld := oldObj.(T)

	if !okNew || !okOld {
		return nil, fmt.Errorf("update validator for custom resource of %T was passed types of %T and %T", h, newObj, oldObj)
	}

	return h.ValidateUpdateFunc(ctx, oldResource, newResource)
}

func (h *Hook[T]) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	if h.ValidateDeleteFunc == nil {
		return nil, nil
	}

	defer h.observe(ctx, "delete", obj)()

	resource, ok := obj.(T)

	if !ok {
		return nil, fmt.Errorf("deletion validator for custom resource of %T was passed type of %T", h, obj)
	}

	return h.ValidateDeleteFunc(ctx, resource)
}

func (h *Hook[T]) observe(ctx context.Context, act string, obj runtime.Object) func() {
	stopTimer := obs.MetricTimerFunc(ctx, "kapi_hook")

	var zeroOfT T
	obs.LogFunc(ctx, 1, "kapi.hook invoked", "type", "kapi_hook_summary", "resource_action", act, "resource_type", fmt.Sprintf("%T", zeroOfT))
	obs.LogFunc(ctx, 3, "kapi.hook invoked", "type", "kapi_hook_trace", "resource_action", act, "resource_type", fmt.Sprintf("%T", zeroOfT), "resource", fmt.Sprintf("+%v", obj))

	return func() {
		stopTimer("resource_type", fmt.Sprintf("%T", zeroOfT), "resource_action", act)
	}
}
