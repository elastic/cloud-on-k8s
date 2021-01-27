// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package fixtures

const (
	AutoscalingResponseSample = `{
  "policies": {
    "di": {
      "required_capacity": {
        "node": {
          "storage": 165155770
        },
        "total": {
          "storage": 3069911040
        }
      },
      "current_capacity": {
        "node": {
          "storage": 1023303680,
          "memory": 2147483648
        },
        "total": {
          "storage": 3069911040,
          "memory": 6442450944
        }
      },
      "current_nodes": [
        {
          "name": "mldi-sample-es-di-0"
        },
        {
          "name": "mldi-sample-es-di-1"
        },
        {
          "name": "mldi-sample-es-di-2"
        }
      ],
      "deciders": {
        "proactive_storage": {
          "required_capacity": {
            "node": {
              "storage": 165155770
            },
            "total": {
              "storage": 3069911040
            }
          },
          "reason_summary": "storage ok",
          "reason_details": {
            "reason": "storage ok",
            "unassigned": 0,
            "assigned": 0,
            "forecasted": 0,
            "forecast_window": "30m"
          }
        },
        "reactive_storage": {
          "required_capacity": {
            "node": {
              "storage": 165155770
            },
            "total": {
              "storage": 3069911040
            }
          },
          "reason_summary": "storage ok",
          "reason_details": {
            "reason": "storage ok",
            "unassigned": 0,
            "assigned": 0
          }
        }
      }
    },
    "ml": {
      "required_capacity": {
        "node": {
          "memory": 3221225472
        },
        "total": {
          "memory": 6442450944
        }
      },
      "current_capacity": {
        "node": {
          "memory": 3221225472
        },
        "total": {
          "memory": 6442450944
        }
      },
      "current_nodes": [
        {
          "name": "mldi-sample-es-ml-0"
        },
        {
          "name": "mldi-sample-es-ml-1"
        }
      ],
      "deciders": {
        "ml": {
          "required_capacity": {
            "node": {
              "memory": 3221225472
            },
            "total": {
              "memory": 6442450944
            }
          },
          "reason_summary": "Passing currently perceived capacity as configured down scale delay has not be satisfied; configured delay [3600000] last detected scale down event [1610452762287]",
          "reason_details": {
            "waiting_analytics_jobs": [],
            "waiting_anomaly_jobs": [],
            "configuration": {},
            "perceived_current_capacity": {
              "node": {
                "memory": 3221225470
              },
              "total": {
                "memory": 6442450940
              }
            },
            "required_capacity": {
              "node": {
                "memory": 251658240
              },
              "total": {
                "memory": 723517440
              }
            },
            "reason": "Passing currently perceived capacity as configured down scale delay has not be satisfied; configured delay [3600000] last detected scale down event [1610452762287]"
          }
        }
      }
    }
  }
}`
)
