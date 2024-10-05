package main

import (
	"context"
	"fmt"
	"time"

	"github.com/comradequinn/kapi"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func addReconcilerExample(ctx context.Context, k *kapi.Cluster) error {

	filterFunc := func(e kapi.ResourceEventType, r client.Object) bool {
		return r.GetName() == "config-data"
	}

	return kapi.AddReconciler(ctx, k, filterFunc, func(ctx context.Context, evt kapi.ReconcileEventType, cfgMap *corev1.ConfigMap) error {

		klient := kapi.ClientFor[*ConfigAudit, *ConfigAuditList](ctx, k, true)

		cfgAudits, err := klient.List(ctx)

		if err != nil {
			return err
		}

		cfgAudit := ConfigAudit{}
		cfgAudit.Name = fmt.Sprintf("configaudit-%v", time.Now().UnixMicro())
		cfgAudit.Namespace = "kapi-example"
		cfgAudit.Spec.Message = fmt.Sprintf("configmap %v created. previous audit count was %v", cfgMap.Name, len(cfgAudits.Items))

		return klient.Create(ctx, &cfgAudit)
	})
}
