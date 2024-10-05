package kapi

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
)

type (
	// ResourceEventType defines the types of events that can affect a k8s resource
	//
	// This is used to limit the invocation of configured ReconcilerFuncs
	ResourceEventType int
	// ReconcileEventType defines the limited, summarised set of event types that can affect a k8s resource as presented to an invoked ReconcilerFunc
	ReconcileEventType int
	// ReconcilerFunc is a generic func that can be added to a kapi.Cluster.
	//
	// It will be invoked with the relevant resource data whenever a resource is created, updated or deleted. When executing as a
	// result of a delete event, `resource T` will be set to its zero-value
	ReconcilerFunc[T client.Object] func(ctx context.Context, eventType ReconcileEventType, resource T) error
	// An ReconcilerFilterFunc is used to reduce the scope of a reconciler. ReconcilerFilterFuncs are invoked before ReconcilerFuncs and are
	//  passed the object being modified and the type of the modification. The associated ReconcilerFunc is only invoked if the eventFilterFunc returns true.
	ReconcilerFilterFunc func(ResourceEventType, client.Object) bool
)

type (
	reconciler[T client.Object] struct {
		reconcilerFunc ReconcilerFunc[T]
		client         *Client[T, *ListUndefined]
	}
)

var (
	ResourceEventTypeCreated = ResourceEventType(0)
	ResourceEventTypeUpdated = ResourceEventType(1)
	ResourceEventTypeDeleted = ResourceEventType(2)
)

var (
	ReconcileEventTypeCreatedOrUpdated = ReconcileEventType(0)
	ReconcileEventTypeDeleted          = ReconcileEventType(1)
)

// AddReconciler causes the specifed ReconcilerFunc to be invoked whenever any resource of type T in the specifed cluster is
// subject to a modification event: create, update or delete.
//
// An eventFilterFunc can be provided that is used to reduce the scope of the reconciler. The eventFilterFunc is invoked before the ReconcilerFunc and is passed the object being
// modified and the type of the modification. The reconcilerFunc is only invoked if the eventFilterFunc returns true.
//
// A nil filterFunc value matches all events.
func AddReconciler[T client.Object](ctx context.Context, cluster *Cluster, reconcilerFilterFunc ReconcilerFilterFunc, reconcilerFunc ReconcilerFunc[T]) error {
	if cluster.connected {
		panic("kapi.add-reconciler must be called before kapi.cluster.connect")
	}

	var resource T

	defer obs.MetricTimerFunc(ctx, "kapi_add_reconciler")("resource_type", fmt.Sprintf("%T", resource))
	obs.LogFunc(ctx, 3, "creating kapi.reconciler", "resource_type", fmt.Sprintf("%T", resource))

	if reconcilerFilterFunc == nil {
		reconcilerFilterFunc = func(_ ResourceEventType, _ client.Object) bool { return true }
	}

	reconcilerFilterFuncWithLogging := func(e ResourceEventType, o client.Object) bool {
		if !reconcilerFilterFunc(e, o) {
			obs.LogFunc(ctx, 3, "kapi.reconciler.filterfunc dropped event", "resource_name", o.GetName(), "resource_namespace", o.GetNamespace(), "resource_type", fmt.Sprintf("%T", resource), "event_type", e.String())
			return false
		}

		obs.LogFunc(ctx, 3, "kapi.reconciler.filterfunc allowed event", "resource_name", o.GetName(), "resource_namespace", o.GetNamespace(), "resource_type", fmt.Sprintf("%T", resource), "event_type", e.String())
		return true
	}

	resource = reflect.New(reflect.TypeOf(resource).Elem()).Interface().(T)

	err := ctrl.NewControllerManagedBy(cluster.manager).
		For(resource).
		WithEventFilter(predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool {
				return reconcilerFilterFuncWithLogging(ResourceEventTypeCreated, e.Object)
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				return reconcilerFilterFuncWithLogging(ResourceEventTypeUpdated, e.ObjectOld)
			},
			DeleteFunc: func(e event.DeleteEvent) bool {
				return reconcilerFilterFuncWithLogging(ResourceEventTypeDeleted, e.Object)
			},
		}).
		Complete(&reconciler[T]{
			reconcilerFunc: reconcilerFunc,
			client:         ClientFor[T, *ListUndefined](ctx, cluster, true),
		})

	if err != nil {
		return fmt.Errorf("unable to configure kapi.reconciler. %v", err)
	}

	obs.LogFunc(ctx, 3, "configured kapi.reconciler", "resource_type", fmt.Sprintf("%T", resource))

	return nil
}

func (r *reconciler[T]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	ctx = obs.NewCorrelationCtx(ctx)

	var (
		resource T
		err      error
	)

	defer obs.MetricTimerFunc(ctx, "kapi_reconcile")("resource_type", fmt.Sprintf("%T", resource))

	evt := ReconcileEventTypeCreatedOrUpdated

	if resource, err = r.client.Get(ctx, req.NamespacedName.Namespace, req.NamespacedName.Name); err != nil {
		if !apierrors.IsNotFound(err) {
			obs.LogFunc(ctx, 0, "kapi.reconciler invoked for invalid resource", "error", err, "resource_name", req.NamespacedName.String(), "resource_type", fmt.Sprintf("%T", resource))
			return ctrl.Result{}, err
		}

		evt = ReconcileEventTypeDeleted
	}

	obs.LogFunc(ctx, 1, "kapi.reconciler invoked", "type", "kapi_reconciler_summary", "resource_name", req.NamespacedName.String(), "resource_type", fmt.Sprintf("%T", resource), "event_type", evt.String())
	obs.LogFunc(ctx, 3, "kapi.reconciler invoked", "type", "kapi_reconciler_trace", "resource_name", req.NamespacedName.String(), "resource_type", fmt.Sprintf("%T", resource), "resource", fmt.Sprintf("%+v", resource), "event_type", evt.String())

	if err := r.reconcilerFunc(ctx, evt, resource); err != nil {
		obs.LogFunc(ctx, 0, "kapi.reconciler unable to invoke reconciler-func", "error", err, "resource_name", req.NamespacedName.String(), "resource_type", fmt.Sprintf("%T", resource), "event_type", evt)
		return ctrl.Result{}, fmt.Errorf("unable to execute configured reconcilerfunc. %v", err)
	}

	return ctrl.Result{}, nil
}

func (r ResourceEventType) String() string {
	switch r {
	case ResourceEventTypeCreated:
		return "created"
	case ResourceEventTypeUpdated:
		return "updated"
	case ResourceEventTypeDeleted:
		return "deleted"
	default:
		return strconv.Itoa(int(r))
	}
}

func (r ReconcileEventType) String() string {
	switch r {
	case ReconcileEventTypeCreatedOrUpdated:
		return "created_or_updated"
	case ReconcileEventTypeDeleted:
		return "deleted"
	default:
		return strconv.Itoa(int(r))
	}
}
