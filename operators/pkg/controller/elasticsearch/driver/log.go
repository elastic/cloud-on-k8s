package driver

import (
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("driver")

func ssetLogger(statefulSet appsv1.StatefulSet) logr.Logger {
	return log.WithValues(
		"namespace", statefulSet.Namespace,
		"statefulset_name", statefulSet.Name,
	)
}
