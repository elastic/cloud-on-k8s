package fixtures

const (
	ErrorSample = `
{
    "status": 400,
    "error": {
        "caused_by": {
            "reason": "cannot set discovery.zen.minimum_master_nodes to more than the current master nodes count [1]",
            "type": "illegal_argument_exception"
        },
        "reason": "illegal value can't update [discovery.zen.minimum_master_nodes] from [1] to [6]",
        "type": "illegal_argument_exception",
        "root_cause": [
            {
                "reason": "[stack-sample-es-575vhzs8ln][10.60.1.22:9300][cluster:admin/settings/update]",
                "type": "remote_transport_exception"
            }
        ]
    }
}
`
)
