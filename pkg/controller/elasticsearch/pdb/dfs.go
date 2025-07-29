// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pdb

import (
	"slices"

	appsv1 "k8s.io/api/apps/v1"

	esv1 "github.com/elastic/cloud-on-k8s/v3/pkg/apis/elasticsearch/v1"
	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
)

var (
	dataRoles = []string{
		string(esv1.DataRole),
		string(esv1.DataHotRole),
		string(esv1.DataWarmRole),
		string(esv1.DataColdRole),
		string(esv1.DataContentRole),
		// Note: DataFrozenRole is excluded as it has different disruption rules (yellow+ health)
	}
)

// normalizeRole returns the normalized form of a role where any data role
// is normalized to the same data role.
func normalizeRole(role string) string {
	if slices.Contains(dataRoles, role) {
		return string(esv1.DataRole)
	}
	return role
}

// groupBySharedRoles groups StatefulSets that share at least one role by first building an adjacency list based
// on shared roles and then using a depth-first search (DFS) to find connected components.
//
// Why an adjacency list?
// 1. It's a simple way to represent connected components.
//
// Example:
// With the following StatefulSets:
// - StatefulSet A (idx 0) with roles ["master", "data"]
// - StatefulSet B (idx 1) with roles ["data_cold"]
// - StatefulSet C (idx 2) with roles ["data"]
// - StatefulSet D (idx 3) with roles ["coordinating"]
//
// The adjacency list would be:
// [
//
//	[1, 2] # sts idx 0 is connected to sts idx 1 and 2
//	[0, 2] # sts idx 1 is connected to sts idx 0 and 2
//	[0, 1] # sts idx 2 is connected to sts idx 0 and 1
//	[]     # sts idx 3 is not connected to any other sts'
//
// ]
//
// Why DFS?
//  1. It's a well known, simple algorithm for traversing or searching tree or graph data structures.
//  2. It's efficient enough for exploring all connected components in a graph.
//     (I believe "union-find" is slightly more efficient, but at this data size it doesn't matter.)
func groupBySharedRoles(statefulSets sset.StatefulSetList) [][]appsv1.StatefulSet {
	n := len(statefulSets)
	if n == 0 {
		return [][]appsv1.StatefulSet{}
	}

	adjList := make([][]int, n)
	roleToIndices := make(map[string][]int)

	// Map roles to StatefulSet indices
	for i, sset := range statefulSets {
		roles := getRolesFromStatefulSetPodTemplate(sset)
		if len(roles) == 0 {
			// StatefulSets with no roles are coordinating nodes - group them together
			roleToIndices["coordinating"] = append(roleToIndices["coordinating"], i)
			continue
		}
		for _, role := range roles {
			normalizedRole := normalizeRole(string(role))
			roleToIndices[normalizedRole] = append(roleToIndices[normalizedRole], i)
		}
	}

	// Populate the adjacency list with each StatefulSet index, and the slice of StatefulSet
	// indices which share roles.
	for _, indices := range roleToIndices {
		for i := 1; i < len(indices); i++ {
			// Connect each StatefulSet to the first StatefulSet with the same role
			// This ensures all StatefulSets with the role are in the same component
			adjList[indices[0]] = append(adjList[indices[0]], indices[i])
			adjList[indices[i]] = append(adjList[indices[i]], indices[0])
		}
	}

	// use iterative DFS (avoiding recursion) to find connected components
	var result [][]appsv1.StatefulSet
	visited := make([]bool, n)

	for i := range statefulSets {
		if visited[i] {
			continue
		}

		group := []appsv1.StatefulSet{}
		stack := []int{i}

		for len(stack) > 0 {
			// Retrieve the top node from the stack
			stsIdx := stack[len(stack)-1]
			// Remove the top node from the stack
			stack = stack[:len(stack)-1]

			if visited[stsIdx] {
				continue
			}

			// Mark statefulSet as visited and add to group
			visited[stsIdx] = true
			group = append(group, statefulSets[stsIdx])

			// Using the adjacency list previously built, push all unvisited statefulSets onto the stack
			// so they are visited on the next iteration.
			for _, neighbor := range adjList[stsIdx] {
				if !visited[neighbor] {
					stack = append(stack, neighbor)
				}
			}
		}

		result = append(result, group)
	}

	return result
}
