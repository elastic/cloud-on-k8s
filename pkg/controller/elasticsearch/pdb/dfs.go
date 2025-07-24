// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License 2.0;
// you may not use this file except in compliance with the Elastic License 2.0.

package pdb

import (
	appsv1 "k8s.io/api/apps/v1"

	"github.com/elastic/cloud-on-k8s/v3/pkg/controller/elasticsearch/sset"
)

// groupBySharedRoles groups StatefulSets that share at least one role using DFS.
func groupBySharedRoles(statefulSets sset.StatefulSetList) [][]appsv1.StatefulSet {
	n := len(statefulSets)
	if n == 0 {
		return [][]appsv1.StatefulSet{}
	}

	// Build adjacency list based on shared roles
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
			roleToIndices[string(role)] = append(roleToIndices[string(role)], i)
		}
	}

	// Create edges between StatefulSets that share any role
	for _, indices := range roleToIndices {
		for i := 1; i < len(indices); i++ {
			// Connect each StatefulSet to the first StatefulSet with the same role
			// This ensures all StatefulSets with the role are in the same component
			adjList[indices[0]] = append(adjList[indices[0]], indices[i])
			adjList[indices[i]] = append(adjList[indices[i]], indices[0])
			// Optionally, connect all pairs for a fully connected component
			for j := 1; j < len(indices); j++ {
				if indices[i] != indices[j] {
					adjList[indices[i]] = append(adjList[indices[i]], indices[j])
					adjList[indices[j]] = append(adjList[indices[j]], indices[i])
				}
			}
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
			// Pop the top node from the stack
			node := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			if visited[node] {
				continue
			}

			// Mark node as visited and add to group
			visited[node] = true
			group = append(group, statefulSets[node])

			// Push all unvisited neighbors onto the stack
			for _, neighbor := range adjList[node] {
				if !visited[neighbor] {
					stack = append(stack, neighbor)
				}
			}
		}

		// Add the group to the result
		result = append(result, group)
	}

	return result
}
