// healthChangeListener returns an OnObservation listener that feeds a generic
// event when a cluster's observed health has changed.
func healthChangeListener(reconciliation chan event.GenericEvent) OnObservation {
	return func(cluster types.NamespacedName, previous State, new State) {
		// no-op if health hasn't change
		if !hasHealthChanged(previous, new) {
			return
		}

		// trigger a reconciliation event for that cluster
		evt := event.GenericEvent{
			Meta: &metav1.ObjectMeta{
				Namespace: cluster.Namespace,
				Name:      cluster.Name,
			},
		}
		reconciliation <- evt
	}
}

// hasHealthChanged returns true if previous and new contain different health.
func hasHealthChanged(previous State, new State) bool {
	switch {
	// both nil
	case previous.ClusterHealth == nil && new.ClusterHealth == nil:
		return false
	// both equal
	case previous.ClusterHealth != nil && new.ClusterHealth != nil &&
		previous.ClusterHealth.Status == new.ClusterHealth.Status:
		return false
	// else: different
	default:
		return true
	}
}
