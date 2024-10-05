package kapi

import (
	"encoding/json"
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type (
	// FieldUndefined is used to indicate that a CustomResource does not define a particular conventional field
	FieldUndefined *struct{}

	// ListUndefined is used to indicate that a CustomResource does not have a list form, or the form is not relevant to the current use-case
	ListUndefined struct {
		CustomResourceList[*CustomResource[FieldUndefined, FieldUndefined, FieldUndefined]]
	}

	// CustomResource defines a template for a struct that represents a K8s CustomResource with the conventional fields of Spec, Status and Scale.
	// The typical use-case is to embed it in a descriptively named struct that represents the CR itself.
	//
	// For example, the below defines an CR named ExampleResource that only exposes a Spec field.
	//
	//	ExampleResource struct {
	//		kapi.CustomResource[ExampleResourceSpec, kapi.FieldUndefined, kapi.FieldUndefined]
	//	}
	//
	//	ExampleResourceSpec struct {
	//		ExampleData string `json:"exampleData"`
	//	}
	//
	// The conventional CR fields are defined as follows:
	//
	//   - 'Spec' defines the main properties of the resource; its desired state.
	//   - 'Status', typically configured as a subresource in the CustomResourceDefinition, defines the current state.
	//   - 'Scale', typically configured as a subresource in the CustomResourceDefinition, defines the scaling properties of the resource.
	//
	// Where any of the above fields are not required, they should be set to the type kapi.FieldUndefined
	CustomResource[TSpec any, TStatus any, TScale any] struct {
		metav1.TypeMeta   `json:",inline"`
		metav1.ObjectMeta `json:"metadata,omitempty"`

		Spec   TSpec   `json:"spec,omitempty"`
		Status TStatus `json:"status,omitempty"`
		Scale  TScale  `json:"scale,omitempty"`
	}

	// CustomResourceList defines a template for the list representation of zero or more CustomResource[T, T, T] items
	CustomResourceList[T client.Object] struct {
		metav1.TypeMeta `json:",inline"`
		metav1.ListMeta `json:"metadata,omitempty"`
		Items           []T `json:"items"`
	}
)

func (e *CustomResourceList[T]) DeepCopyObject() runtime.Object {
	if e == nil {
		return nil
	}

	var out *CustomResourceList[T]
	out = reflect.New(reflect.TypeOf(out).Elem()).Interface().(*CustomResourceList[T])

	b, _ := json.Marshal(e)
	json.Unmarshal(b, out)

	return out
}

func (e *CustomResource[TSpec, TStatus, TScale]) DeepCopyObject() runtime.Object {
	if e == nil {
		return nil
	}

	var out *CustomResource[TSpec, TStatus, TScale]
	out = reflect.New(reflect.TypeOf(out).Elem()).Interface().(*CustomResource[TSpec, TStatus, TScale])

	b, _ := json.Marshal(e)
	json.Unmarshal(b, out)

	return out
}
