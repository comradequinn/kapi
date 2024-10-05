package kapi

import (
	"context"
	"log"
	"os"
	"os/exec"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type (
	TestResourceSpec struct {
		TestData string `json:"testData"`
	}
	TestResource     = CustomResource[TestResourceSpec, FieldUndefined, FieldUndefined]
	TestResourceList = CustomResourceList[*TestResource]
)

var (
	testCluster        = "kapi-test"
	testNamespace      = "kapi-test"
	cluster            *Cluster
	ctx                = context.Background()
	reconcilerExecuted = make(chan struct{}, 1)
)

func TestMain(m *testing.M) {
	mustHaveBinary("kind")
	mustHaveBinary("kubectl")

	execCmd(true, "kind", "delete", "clusters", testCluster)
	execCmd(true, "kind", "create", "cluster", "--name", testCluster)
	execKubectl(true, false, "create", "namespace", testNamespace)

	Init(ObservabilityConfig{
		BackgroundContext: ctx,
		LogFunc: func(ctx context.Context, level int, msg string, attributes ...any) {
			if level == 0 {
				log.Printf("%v. attributes: %+v", msg, attributes)
			}
		},
		MetricTimerFunc: func(ctx context.Context, metric string) func(attributes ...string) {
			return func(attributes ...string) {}
		},
		NewCorrelationCtx: func(ctx context.Context) context.Context {
			return ctx
		},
	})

	var err error
	cluster, err = NewCluster(ctx, ClusterConfig{
		LeaderElection: LeaderElectionConfig{
			Enabled:      true,
			LockResource: "kapi-test-leader-election-lock",
		},
		Namespaces: []string{
			testNamespace,
		},
		CRDs: []CRDs{
			{
				APIGroup:   "kapi-test.comradequinn.github.io",
				APIVersion: "v1",
				Kinds: map[string]KindType{
					"TestResource":     &TestResource{},
					"TestResourceList": &TestResourceList{},
				},
			},
		},
	})

	if err != nil {
		log.Fatalf("error creating kapi.cluster: %v", err)
	}

	filterFunc := func(e ResourceEventType, r client.Object) bool {
		return e == ResourceEventTypeCreated && r.GetName() == "test-data"
	}

	err = AddReconciler(ctx, cluster, filterFunc, func(ctx context.Context, evt ReconcileEventType, resource *corev1.ConfigMap) error {
		if resource.GetName() == "test-data" {
			close(reconcilerExecuted)
		}
		return nil
	})

	if err != nil {
		log.Fatalf("error creating kapi.cluster: %v", err)
	}

	go func() {
		if err := cluster.Connect(ctx); err != nil {
			log.Fatalf("error connecting to cluster: %v", err)
		}
	}()

	<-time.After(time.Second * 5) // wait for cluster to be ready

	code := m.Run()

	execCmd(true, "kind", "delete", "clusters", testCluster)

	os.Exit(code)
}

func TestKapiClient(t *testing.T) {
	klient := ClientFor[*corev1.ConfigMap, *corev1.ConfigMapList](ctx, cluster, false)

	expectedKey, expectedValue := "key1", "value1"
	cfgMap := &corev1.ConfigMap{}
	cfgMap.Name = "test-data"
	cfgMap.Namespace = testNamespace
	cfgMap.Data = map[string]string{
		expectedKey: expectedValue + "some other data to be removed during update",
	}

	var err error

	if err = klient.Create(ctx, cfgMap); err != nil {
		t.Fatalf("expected no error creating configmap, got: %v", err)
	}

	cfgMap, err = klient.Get(ctx, cfgMap.Namespace, cfgMap.Name)

	if err != nil {
		t.Fatalf("expected no error getting configmap, got: %v", err)
	}

	cfgMap.Data[expectedKey] = expectedValue

	if err = klient.Update(ctx, cfgMap); err != nil {
		t.Fatalf("expected no error updating configmap, got: %v", err)
	}

	cfgMap, err = klient.Get(ctx, cfgMap.Namespace, cfgMap.Name)

	if err != nil {
		t.Fatalf("expected no error getting configmap, got: %v", err)
	}

	if cfgMap.Data[expectedKey] != expectedValue {
		t.Fatalf("expected value %v for key %v after update, got: %v", expectedValue, expectedKey, cfgMap.Data[expectedKey])
	}

	configMaps, err := klient.List(ctx)

	if err != nil {
		t.Fatalf("expected no error listing configmaps, got: %v", err)
	}

	configMapCount := len(configMaps.Items)

	if err = klient.Delete(ctx, cfgMap); err != nil {
		t.Fatalf("expected no error deleting configmap, got: %v", err)
	}

	configMaps, err = klient.List(ctx)

	if err != nil {
		t.Fatalf("expected no error re-listing configmaps, got: %v", err)
	}

	if len(configMaps.Items) != configMapCount-1 {
		t.Fatalf("expected %v configmaps after deletion, got: %v", configMapCount-1, len(configMaps.Items))
	}
}

func TestReconciler(t *testing.T) {
	// the testmain func configures a reconciler that should be triggered by the client tests; when it is, it closes the reconcilerExecuted channel
	select {
	case <-reconcilerExecuted:
	case <-time.After(time.Second * 30):
		t.Fatalf("reconciler did not execute")
	}
}

func TestCRD(t *testing.T) {
	crd := apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testresources.kapi-test.comradequinn.github.io",
		},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "kapi-test.comradequinn.github.io",
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &apiextensionsv1.CustomResourceValidation{
						OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]apiextensionsv1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]apiextensionsv1.JSONSchemaProps{
										"testData": {Type: "string"},
									},
								},
							},
						},
					},
				},
			},
			Scope: apiextensionsv1.NamespaceScoped,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Plural:   "testresources",
				Singular: "testresource",
				Kind:     "TestResource",
			},
		},
	}

	crdKlient := ClientFor[*apiextensionsv1.CustomResourceDefinition, *apiextensionsv1.CustomResourceDefinitionList](ctx, cluster, true)

	if err := crdKlient.Create(ctx, &crd); err != nil {
		t.Fatalf("expected no error creating custom resource definition, got: %v", err)
	}

	{ // poll for the CRD to be established for up to 30 seconds (expected time should be <1s)
		crdReady := false

		for range 6 {
			crds, err := crdKlient.List(ctx)

			if err != nil {
				t.Fatalf("expected no error listing custom resource definitions, got: %v", err)
			}

			if len(crds.Items) == 1 {
				crdReady = true
				break
			}
			<-time.After(time.Second * 5)
		}

		if !crdReady {
			t.Fatalf("expected custom resource definition to be ready")
		}
	}

	klient := ClientFor[*TestResource, *TestResourceList](ctx, cluster, false)

	expectedTestResource := TestResource{
		Spec: TestResourceSpec{
			TestData: "test data",
		},
	}

	expectedTestResource.Name = "test-resource"
	expectedTestResource.Namespace = testNamespace

	if err := klient.Create(ctx, &expectedTestResource); err != nil {
		t.Fatalf("expected no error creating custom resource, got: %v", err)
	}

	testResource, err := klient.Get(ctx, expectedTestResource.Namespace, expectedTestResource.Name)

	if err != nil {
		t.Fatalf("expected no error getting custom resource, got: %v", err)
	}

	if testResource.Spec.TestData != expectedTestResource.Spec.TestData {
		t.Fatalf("expected test data %v, got: %v", expectedTestResource.Spec.TestData, testResource.Spec.TestData)
	}

	testResources, err := klient.List(ctx)

	if err != nil {
		t.Fatalf("expected no error getting custom resource list, got: %v", err)
	}

	if len(testResources.Items) != 1 {
		t.Fatalf("expected 1 custom resource in list items, got: %v", len(testResources.Items))
	}

	if testResources.Items[0].Spec.TestData != expectedTestResource.Spec.TestData {
		t.Fatalf("expected list[0] test data %v, got: %v", expectedTestResource.Spec.TestData, testResource.Spec.TestData)
	}
}

func mustHaveBinary(name string) {
	if _, err := exec.LookPath(name); err != nil {
		log.Fatalf("%v binary not found", name)
	}
}

func execCmd(must bool, cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	if err := c.Run(); err != nil {
		if must {
			log.Fatalf("error executing %s: %v", c.String(), err)
		}
		return err
	}
	return nil
}

func execKubectl(must, includeNamespace bool, args ...string) error {
	args = append(args, "--context", "kind-"+testCluster)

	if includeNamespace {
		args = append(args, "--namespace", testNamespace)
	}

	c := exec.Command("kubectl", args...)
	if err := c.Run(); err != nil {
		if must {
			log.Fatalf("error executing %s: %v", c.String(), err)
		}
		return err
	}
	return nil
}
