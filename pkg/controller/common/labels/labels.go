package labels

import (
	k8slabels "k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TODO sabo remove this once we can convert all callers
func SelectorToMatchingLabels(selector k8slabels.Selector) client.MatchingLabels {

	reqs, _ := selector.Requirements()
	labelReqs := make(map[string]string)
	for _, req := range reqs {
		// only accept single value requirements
		// length := req.Values().Len()
		if req.Values().Len() > 0 {
			labelReqs[req.Key()] = req.Values().List()[0]
		}
	}
	return client.MatchingLabels(labelReqs)
}
