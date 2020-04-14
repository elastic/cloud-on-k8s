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
