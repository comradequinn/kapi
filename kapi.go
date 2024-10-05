package kapi

import (
	"context"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/comradequinn/kapi/internal/logconv"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type (
	// Cluster represents a k8s cluster. A kapi.Cluster is used to
	// - configure one or more ReconcilerFuncs that are executed when specified k8s cluster resource-change events occur
	// - access a `client` that can be used to perform resource level CRUD operations against a k8s cluster
	Cluster struct {
		manager   manager.Manager
		connected bool
	}
	// ClusterConfig defines information about how to interact with a specific k8s cluster
	ClusterConfig struct {
		// TLS defines the directory in which the TLS certificates to use when serving any configured hooks are stored
		TLS string
		// DisableCaching disables caching of cluster information locally.
		//
		// This canis typically used in scenarios where resources are modified outside of the kapi.Cluster scope or where
		// resource modifications need to be reflected immediately
		DisableCaching bool
		// LeaderElectionConfig defines the configuration for leader election. By default it is disabled.
		// Enabling leader election allows multiple replicas of a kapi based controller or operator to be deployed to support high availabity
		LeaderElection LeaderElectionConfig
		// Namespaces defines the namespaces for which to invoke configured Reconcilers
		// An empty slice results in applicable Reconcilers being invoked for all namespaces
		Namespaces []string
		// CRDs defines any CRDs that the Cluster must recognise
		CRDs []CRDs
	}
	// LeaderElectionConfig defines the configuration for the leader elections of high availablity deployments
	LeaderElectionConfig struct {
		// Enabled enables leader election for kpai based controllers or operators running multiple replicas to support high availabity
		Enabled bool
		// LockResource defines the name of the resource that will be used as the lock resource in leader elections.
		// This should be set to a unique value for the consuming application to avoid conflicts with other application's leader elections
		LockResource string
		// Namespace defines the namespace in which the leader election resource will be created.
		// This can be unset if the cluster.Config namespace is set to one or more namespaces, which will result in the first namespace in the list being used
		Namespace string
	}
	// CRDs defines a mapping between a set of one or more structs that each represent a CRD and the k8s API Group and Version that they are defined within
	CRDs struct {
		APIGroup   string
		APIVersion string
		Kinds      map[string]KindType
	}
	// KindType is an interface that is implemented by any type that is based on kapi.CustomResource or kapi.CustomResourceLlist
	KindType runtime.Object
)

var (
	obs ObservabilityConfig
)

// Init configures observability for all kapi.Clusters
func Init(cfg ObservabilityConfig) {
	if cfg.LogFunc == nil || cfg.MetricTimerFunc == nil || cfg.NewCorrelationCtx == nil || cfg.BackgroundContext == nil {
		panic("kapi.init called with observabilityconfig with nil logfunc, metrictimerfunc, newcorrelationctx or backgroundcontext")
	}

	lf := cfg.LogFunc
	cfg.LogFunc = func(ctx context.Context, level int, msg string, attributes ...any) {
		lf(ctx, level, msg, append(attributes, "lib", "kapi")...)
	}

	obs = cfg
	ctrl.SetLogger(logconv.NewLogrWrapper(cfg.BackgroundContext, cfg.LogFunc))
}

// NewCluster returns a new kapi.Cluster based on the passed kapi.ClusterConfig
//
// Multiple kapi.Clusters can be created to manage different configurations. However, it
// is preferable and typical to re-use the same kapi.Cluster where possible as this
// improves cache efficiency.
//
// For example, where possible, create one kapi.Cluster and add two Reconcilers as opposed to
// creating two kapi.Clusters with one Reconciler attached to each
func NewCluster(ctx context.Context, cfg ClusterConfig) (*Cluster, error) {
	defer obs.MetricTimerFunc(ctx, "kapi_new_cluster")()

	if cfg.LeaderElection.Enabled {
		if cfg.LeaderElection.LockResource == "" {
			panic("a leader-election-resource must be set, unless leader elections are disabled")
		}

		if cfg.LeaderElection.Namespace == "" {
			if len(cfg.Namespaces) == 0 {
				panic("a namespace must be set, unless leader elections are disabled")
			}
			cfg.LeaderElection.Namespace = cfg.Namespaces[0]
		}
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))

	for crd := range slices.Values(cfg.CRDs) {
		schemeBuilder := runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
			gv := (schema.GroupVersion{
				Group:   crd.APIGroup,
				Version: crd.APIVersion,
			})

			metav1.AddToGroupVersion(scheme, gv)

			for kindName, kindType := range maps.All(crd.Kinds) {
				gvk := gv.WithKind(kindName)
				obs.LogFunc(ctx, 3, "registering kind type mapping in scheme", "gvk", gvk.String(), "kind_type", reflect.TypeOf(kindType).Elem().Name())
				scheme.AddKnownTypeWithName(gvk, kindType)
			}

			return nil
		})

		schemeBuilder.AddToScheme(scheme)
	}

	namespaces := make(map[string]cache.Config, len(cfg.Namespaces))

	for ns := range slices.Values(cfg.Namespaces) {
		namespaces[ns] = cache.Config{}
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), manager.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: "0",
		},
		Cache: cache.Options{
			DefaultNamespaces: namespaces,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			CertDir: cfg.TLS,
		}),

		LeaderElection:          cfg.LeaderElection.Enabled,
		LeaderElectionID:        cfg.LeaderElection.LockResource,
		LeaderElectionNamespace: cfg.LeaderElection.Namespace,
	})

	if err != nil {
		return nil, fmt.Errorf("unable to create controller manager for kapi.cluster. %v", err)
	}

	obs.LogFunc(ctx, 3, "created kapi.cluster", "namespaces", cfg.Namespaces)

	return &Cluster{
		manager: mgr,
	}, nil
}

// Connect starts all configured Reconcilers and enables the use of Clients
func (cluster *Cluster) Connect(ctx context.Context) error {
	defer obs.MetricTimerFunc(ctx, "kapi_connect")()

	if cluster.connected {
		panic("kapi.cluster.connect called more than once")
	}

	cluster.connected = true

	obs.LogFunc(ctx, 3, "connecting k8s.cluster")

	if err := cluster.manager.Start(ctx); err != nil {
		return fmt.Errorf("unable to start controller-runtime.manager for kapi.cluster. %v", err)
	}

	return nil
}
