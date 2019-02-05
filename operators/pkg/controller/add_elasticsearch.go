package controller

import (
	"github.com/elastic/k8s-operators/operators/pkg/controller/elasticsearch"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, elasticsearch.Add)
}
