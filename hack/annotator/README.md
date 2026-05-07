Annotator Script
================

This script provides utilities for adding, removing, and listing annotations to/from Elastic resources deployed in a Kubernetes cluster.


Usage:

```
# Add the my.domain/annotation=value annotation to all Elastic resources
ANN_KEY="my.domain/annotation" ANN_VAL="value" ./annotator.sh add

# List all Elastic resources that have the my.domain/annotation set
ANN_KEY="my.domain/annotation" ./annotator.sh ls

# Remove the my.domain/annotation from all Elastic resources
ANN_KEY="my.domain/annotation" PAUSE_SECS=10 ./annotator.sh remove
```

Usage of this script with the `ANN_KEY` value of `eck.k8s.elastic.co/managed` is deprecated in favor of setting the value of the `eck.k8s.elastic.co/pause-orchestration` annotation.
