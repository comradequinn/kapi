package main

import "github.com/comradequinn/kapi"

type (
	ConfigAudit     = kapi.CustomResource[ConfigAuditSpec, kapi.FieldUndefined, kapi.FieldUndefined]
	ConfigAuditList = kapi.CustomResourceList[*ConfigAudit]

	ConfigAuditSpec struct {
		Message string `json:"message"`
	}
)
