package fixtures

const (
	NodesSample = `
	{
		"_nodes": {
		  "successful": 3,
		  "failed": 0,
		  "total": 3
		},
		"cluster_name": "af932d24216a4dd69ba47d2fd3214796",
		"nodes": {
		  "iXqjbgPYThO-6S7reL5_HA": {
			"plugins": [
			  {
				"has_native_controller": false,
				"description": "Elasticsearch plugin for Found",
				"java_version": "1.8",
				"classname": "org.elasticsearch.plugin.found.FoundPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "found-elasticsearch"
			  },
			  {
				"has_native_controller": false,
				"description": "Ingest processor that uses looksup geo data based on ip adresses using the Maxmind geo database",
				"java_version": "1.8",
				"classname": "org.elasticsearch.ingest.geoip.IngestGeoIpPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "ingest-geoip"
			  },
			  {
				"has_native_controller": false,
				"description": "Ingest processor that extracts information from a user agent",
				"java_version": "1.8",
				"classname": "org.elasticsearch.ingest.useragent.IngestUserAgentPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "ingest-user-agent"
			  },
			  {
				"has_native_controller": false,
				"description": "The S3 repository plugin adds S3 repositories",
				"java_version": "1.8",
				"classname": "org.elasticsearch.repositories.s3.S3RepositoryPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "repository-s3"
			  }
			],
			"thread_pool": {
			  "ml_utility": {
				"max": 80,
				"queue_size": 500,
				"type": "fixed",
				"min": 80
			  },
			  "index": {
				"max": 2,
				"queue_size": 200,
				"type": "fixed",
				"min": 2
			  },
			  "search": {
				"max": 4,
				"queue_size": 1000,
				"type": "fixed_auto_queue_size",
				"min": 4
			  },
			  "force_merge": {
				"max": 1,
				"queue_size": -1,
				"type": "fixed",
				"min": 1
			  },
			  "get": {
				"max": 2,
				"queue_size": 1000,
				"type": "fixed",
				"min": 2
			  },
			  "generic": {
				"max": 128,
				"queue_size": -1,
				"keep_alive": "30s",
				"type": "scaling",
				"min": 4
			  },
			  "analyze": {
				"max": 1,
				"queue_size": 16,
				"type": "fixed",
				"min": 1
			  },
			  "write": {
				"max": 2,
				"queue_size": 200,
				"type": "fixed",
				"min": 2
			  },
			  "refresh": {
				"max": 1,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "watcher": {
				"max": 10,
				"queue_size": 1000,
				"type": "fixed",
				"min": 10
			  },
			  "listener": {
				"max": 1,
				"queue_size": -1,
				"type": "fixed",
				"min": 1
			  },
			  "rollup_indexing": {
				"max": 4,
				"queue_size": 4,
				"type": "fixed",
				"min": 4
			  },
			  "management": {
				"max": 5,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "warmer": {
				"max": 1,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "flush": {
				"max": 1,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "fetch_shard_started": {
				"max": 4,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "snapshot": {
				"max": 1,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "security-token-key": {
				"max": 1,
				"queue_size": 1000,
				"type": "fixed",
				"min": 1
			  },
			  "ml_autodetect": {
				"max": 80,
				"queue_size": 80,
				"type": "fixed",
				"min": 80
			  },
			  "ml_datafeed": {
				"max": 20,
				"queue_size": 200,
				"type": "fixed",
				"min": 20
			  },
			  "fetch_shard_store": {
				"max": 4,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  }
			},
			"transport_address": "172.25.98.100:19850",
			"http": {
			  "max_content_length_in_bytes": 104857600,
			  "publish_address": "172.25.98.100:18367",
			  "bound_address": [
				"172.17.0.34:18367"
			  ]
			},
			"name": "instance-0000000007",
			"roles": [
			  "master",
			  "data",
			  "ingest"
			],
			"total_indexing_buffer": 213005107,
			"process": {
			  "mlockall": false,
			  "id": 915,
			  "refresh_interval_in_millis": 1000
			},
			"ip": "172.25.98.100",
			"settings": {
			  "node": {
				"attr": {
				  "region": "us-east-1",
				  "logical_availability_zone": "zone-1",
				  "instance_configuration": "aws.data.highio.i3",
				  "xpack": {
					"installed": "true"
				  },
				  "availability_zone": "us-east-1b"
				},
				"ml": "false",
				"ingest": "true",
				"name": "instance-0000000007",
				"master": "true",
				"data": "true",
				"max_local_storage_nodes": "1"
			  },
			  "reindex": {
				"remote": {
				  "whitelist": [
					"*.io:*",
					"*.com:*"
				  ]
				}
			  },
			  "processors": "2",
			  "xpack": {
				"notification": {
				  "email": {
					"account": {
					  "work": {
						"email_defaults": {
						  "from": "Watcher Alert <noreply@watcheralert.found.io>"
						},
						"smtp": {
						  "host": "dockerhost",
						  "port": "10025"
						}
					  }
					}
				  }
				},
				"security": {
				  "authc": {
					"token": {
					  "enabled": "true"
					},
					"reserved_realm": {
					  "enabled": "false"
					},
					"realms": {
					  "found": {
						"type": "file",
						"order": "0"
					  },
					  "native": {
						"type": "native",
						"order": "1"
					  }
					},
					"anonymous": {
					  "username": "anonymous",
					  "authz_exception": "false",
					  "roles": "anonymous"
					}
				  },
				  "enabled": "true",
				  "http": {
					"ssl": {
					  "enabled": "true"
					}
				  },
				  "transport": {
					"ssl": {
					  "enabled": "true"
					}
				  }
				},
				"monitoring": {
				  "collection": {
					"enabled": "false"
				  },
				  "history": {
					"duration": "3d"
				  }
				},
				"license": {
				  "self_generated": {
					"type": "trial"
				  }
				},
				"ml": {
				  "enabled": "true"
				}
			  },
			  "script": {
				"allowed_types": "stored,inline"
			  },
			  "s3": {
				"client": {
				  "default": {
					"endpoint": "s3.amazonaws.com"
				  }
				}
			  },
			  "cluster": {
				"indices": {
				  "close": {
					"enable": "false"
				  }
				},
				"name": "af932d24216a4dd69ba47d2fd3214796",
				"routing": {
				  "allocation": {
					"disk": {
					  "threshold_enabled": "false"
					},
					"awareness": {
					  "attributes": "region,availability_zone,logical_availability_zone"
					}
				  }
				}
			  },
			  "client": {
				"type": "node"
			  },
			  "action": {
				"auto_create_index": "true",
				"destructive_requires_name": "false"
			  },
			  "pidfile": "/app/es.pid",
			  "discovery": {
				"zen": {
				  "minimum_master_nodes": "3",
				  "ping": {
					"unicast": {
					  "hosts": "172.25.137.90:19338,172.25.85.111:19673,172.25.44.68:19866,172.25.133.112:19447"
					}
				  },
				  "hosts_provider": "found"
				}
			  }
			},
			"modules": [
			  {
				"has_native_controller": false,
				"description": "Adds aggregations whose input are a list of numeric fields and output includes a matrix.",
				"java_version": "1.8",
				"classname": "org.elasticsearch.search.aggregations.matrix.MatrixAggregationPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "aggs-matrix-stats"
			  },
			  {
				"has_native_controller": false,
				"description": "Adds \"built in\" analyzers to Elasticsearch.",
				"java_version": "1.8",
				"classname": "org.elasticsearch.analysis.common.CommonAnalysisPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "analysis-common"
			  },
			  {
				"has_native_controller": false,
				"description": "Module for ingest processors that do not require additional security permissions or have large dependencies and resources",
				"java_version": "1.8",
				"classname": "org.elasticsearch.ingest.common.IngestCommonPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "lang-painless"
				],
				"name": "ingest-common"
			  },
			  {
				"has_native_controller": false,
				"description": "Lucene expressions integration for Elasticsearch",
				"java_version": "1.8",
				"classname": "org.elasticsearch.script.expression.ExpressionPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "lang-expression"
			  },
			  {
				"has_native_controller": false,
				"description": "Mustache scripting integration for Elasticsearch",
				"java_version": "1.8",
				"classname": "org.elasticsearch.script.mustache.MustachePlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "lang-mustache"
			  },
			  {
				"has_native_controller": false,
				"description": "An easy, safe and fast scripting language for Elasticsearch",
				"java_version": "1.8",
				"classname": "org.elasticsearch.painless.PainlessPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "lang-painless"
			  },
			  {
				"has_native_controller": false,
				"description": "Adds advanced field mappers",
				"java_version": "1.8",
				"classname": "org.elasticsearch.index.mapper.MapperExtrasPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "mapper-extras"
			  },
			  {
				"has_native_controller": false,
				"description": "This module adds the support parent-child queries and aggregations",
				"java_version": "1.8",
				"classname": "org.elasticsearch.join.ParentJoinPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "parent-join"
			  },
			  {
				"has_native_controller": false,
				"description": "Percolator module adds capability to index queries and query these queries by specifying documents",
				"java_version": "1.8",
				"classname": "org.elasticsearch.percolator.PercolatorPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "percolator"
			  },
			  {
				"has_native_controller": false,
				"description": "The Rank Eval module adds APIs to evaluate ranking quality.",
				"java_version": "1.8",
				"classname": "org.elasticsearch.index.rankeval.RankEvalPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "rank-eval"
			  },
			  {
				"has_native_controller": false,
				"description": "The Reindex module adds APIs to reindex from one index to another or update documents in place.",
				"java_version": "1.8",
				"classname": "org.elasticsearch.index.reindex.ReindexPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "reindex"
			  },
			  {
				"has_native_controller": false,
				"description": "Module for URL repository",
				"java_version": "1.8",
				"classname": "org.elasticsearch.plugin.repository.url.URLRepositoryPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "repository-url"
			  },
			  {
				"has_native_controller": false,
				"description": "Netty 4 based transport implementation",
				"java_version": "1.8",
				"classname": "org.elasticsearch.transport.Netty4Plugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "transport-netty4"
			  },
			  {
				"has_native_controller": false,
				"description": "Tribe module",
				"java_version": "1.8",
				"classname": "org.elasticsearch.tribe.TribePlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "tribe"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Core",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.core.XPackPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "x-pack-core"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Deprecation",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.deprecation.Deprecation",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-deprecation"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Graph",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.graph.Graph",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-graph"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Logstash",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.logstash.Logstash",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-logstash"
			  },
			  {
				"has_native_controller": true,
				"description": "Elasticsearch Expanded Pack Plugin - Machine Learning",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.ml.MachineLearning",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-ml"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Monitoring",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.monitoring.Monitoring",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-monitoring"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Rollup",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.rollup.Rollup",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-rollup"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Security",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.security.Security",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-security"
			  },
			  {
				"has_native_controller": false,
				"description": "The Elasticsearch plugin that powers SQL for Elasticsearch",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.sql.plugin.SqlPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core",
				  "lang-painless"
				],
				"name": "x-pack-sql"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Upgrade",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.upgrade.Upgrade",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-upgrade"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Watcher",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.watcher.Watcher",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-watcher"
			  }
			],
			"ingest": {
			  "processors": [
				{
				  "type": "append"
				},
				{
				  "type": "bytes"
				},
				{
				  "type": "convert"
				},
				{
				  "type": "date"
				},
				{
				  "type": "date_index_name"
				},
				{
				  "type": "dot_expander"
				},
				{
				  "type": "fail"
				},
				{
				  "type": "foreach"
				},
				{
				  "type": "geoip"
				},
				{
				  "type": "grok"
				},
				{
				  "type": "gsub"
				},
				{
				  "type": "join"
				},
				{
				  "type": "json"
				},
				{
				  "type": "kv"
				},
				{
				  "type": "lowercase"
				},
				{
				  "type": "remove"
				},
				{
				  "type": "rename"
				},
				{
				  "type": "script"
				},
				{
				  "type": "set"
				},
				{
				  "type": "set_security_user"
				},
				{
				  "type": "sort"
				},
				{
				  "type": "split"
				},
				{
				  "type": "trim"
				},
				{
				  "type": "uppercase"
				},
				{
				  "type": "urldecode"
				},
				{
				  "type": "user_agent"
				}
			  ]
			},
			"host": "172.25.98.100",
			"version": "6.4.1",
			"jvm": {
			  "vm_name": "Java HotSpot(TM) 64-Bit Server VM",
			  "vm_version": "25.144-b01",
			  "mem": {
				"non_heap_max_in_bytes": 0,
				"heap_init_in_bytes": 2147483648,
				"heap_max_in_bytes": 2130051072,
				"direct_max_in_bytes": 2130051072,
				"non_heap_init_in_bytes": 2555904
			  },
			  "gc_collectors": [
				"ParNew",
				"ConcurrentMarkSweep"
			  ],
			  "using_compressed_ordinary_object_pointers": "true",
			  "pid": 915,
			  "input_arguments": [
				"-XX:+UseConcMarkSweepGC",
				"-XX:CMSInitiatingOccupancyFraction=75",
				"-XX:+UseCMSInitiatingOccupancyOnly",
				"-XX:+AlwaysPreTouch",
				"-Xss1m",
				"-Djava.awt.headless=true",
				"-Dfile.encoding=UTF-8",
				"-Djna.nosys=true",
				"-XX:-OmitStackTraceInFastThrow",
				"-Dio.netty.noUnsafe=true",
				"-Dio.netty.noKeySetOptimization=true",
				"-Dio.netty.recycler.maxCapacityPerThread=0",
				"-Dlog4j.shutdownHookEnabled=false",
				"-Dlog4j2.disable.jmx=true",
				"-Djava.io.tmpdir=/tmp/elasticsearch.9FgKkEyP",
				"-XX:HeapDumpPath=data",
				"-XX:ErrorFile=logs/hs_err_pid%p.log",
				"-XX:+PrintGCDetails",
				"-XX:+PrintGCDateStamps",
				"-XX:+PrintTenuringDistribution",
				"-XX:+PrintGCApplicationStoppedTime",
				"-Xloggc:/app/logs/gc.log",
				"-XX:+UseGCLogFileRotation",
				"-XX:NumberOfGCLogFiles=2",
				"-XX:GCLogFileSize=8m",
				"-Des.allow_insecure_settings=true",
				"-XX:ParallelGCThreads=2",
				"-XX:ConcGCThreads=1",
				"-Xms2048M",
				"-Xmx2048M",
				"-Djava.nio.file.spi.DefaultFileSystemProvider=co.elastic.cloud.quotaawarefs.QuotaAwareFileSystemProvider",
				"-Dcurator-log-only-first-connection-issue-as-error-level=true",
				"-Dio.netty.recycler.maxCapacityPerThread=0",
				"-Djava.security.policy=file:///app/config/gelf.policy",
				"-Des.cgroups.hierarchy.override=/",
				"-Des.geoip.load_db_on_heap=true",
				"-Des.path.home=/elasticsearch",
				"-Des.path.conf=/app/config",
				"-Des.distribution.flavor=default",
				"-Des.distribution.type=tar"
			  ],
			  "version": "1.8.0_144",
			  "vm_vendor": "Oracle Corporation",
			  "memory_pools": [
				"Code Cache",
				"Metaspace",
				"Compressed Class Space",
				"Par Eden Space",
				"Par Survivor Space",
				"CMS Old Gen"
			  ],
			  "start_time_in_millis": 1543482103166
			},
			"build_flavor": "default",
			"build_hash": "e36acdb",
			"attributes": {
			  "instance_configuration": "aws.data.highio.i3",
			  "region": "us-east-1",
			  "logical_availability_zone": "zone-1",
			  "xpack.installed": "true",
			  "availability_zone": "us-east-1b"
			},
			"os": {
			  "name": "Linux",
			  "allocated_processors": 2,
			  "version": "4.4.0-1048-aws",
			  "arch": "amd64",
			  "refresh_interval_in_millis": 1000,
			  "available_processors": 32
			},
			"build_type": "tar",
			"transport": {
			  "publish_address": "172.25.98.100:19850",
			  "bound_address": [
				"172.17.0.34:19850"
			  ],
			  "profiles": {
				"client": {
				  "publish_address": "172.17.0.34:20705",
				  "bound_address": [
					"172.17.0.34:20705"
				  ]
				}
			  }
			}
		  },
		  "EwsDTq-KSny1gbUcH77nxA": {
			"plugins": [
			  {
				"has_native_controller": false,
				"description": "Elasticsearch plugin for Found",
				"java_version": "1.8",
				"classname": "org.elasticsearch.plugin.found.FoundPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "found-elasticsearch"
			  },
			  {
				"has_native_controller": false,
				"description": "Ingest processor that uses looksup geo data based on ip adresses using the Maxmind geo database",
				"java_version": "1.8",
				"classname": "org.elasticsearch.ingest.geoip.IngestGeoIpPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "ingest-geoip"
			  },
			  {
				"has_native_controller": false,
				"description": "Ingest processor that extracts information from a user agent",
				"java_version": "1.8",
				"classname": "org.elasticsearch.ingest.useragent.IngestUserAgentPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "ingest-user-agent"
			  },
			  {
				"has_native_controller": false,
				"description": "The S3 repository plugin adds S3 repositories",
				"java_version": "1.8",
				"classname": "org.elasticsearch.repositories.s3.S3RepositoryPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "repository-s3"
			  }
			],
			"thread_pool": {
			  "ml_utility": {
				"max": 80,
				"queue_size": 500,
				"type": "fixed",
				"min": 80
			  },
			  "index": {
				"max": 2,
				"queue_size": 200,
				"type": "fixed",
				"min": 2
			  },
			  "search": {
				"max": 4,
				"queue_size": 1000,
				"type": "fixed_auto_queue_size",
				"min": 4
			  },
			  "force_merge": {
				"max": 1,
				"queue_size": -1,
				"type": "fixed",
				"min": 1
			  },
			  "get": {
				"max": 2,
				"queue_size": 1000,
				"type": "fixed",
				"min": 2
			  },
			  "generic": {
				"max": 128,
				"queue_size": -1,
				"keep_alive": "30s",
				"type": "scaling",
				"min": 4
			  },
			  "analyze": {
				"max": 1,
				"queue_size": 16,
				"type": "fixed",
				"min": 1
			  },
			  "write": {
				"max": 2,
				"queue_size": 200,
				"type": "fixed",
				"min": 2
			  },
			  "refresh": {
				"max": 1,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "watcher": {
				"max": 1,
				"queue_size": 1000,
				"type": "fixed",
				"min": 1
			  },
			  "listener": {
				"max": 1,
				"queue_size": -1,
				"type": "fixed",
				"min": 1
			  },
			  "rollup_indexing": {
				"max": 4,
				"queue_size": 4,
				"type": "fixed",
				"min": 4
			  },
			  "management": {
				"max": 5,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "warmer": {
				"max": 1,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "flush": {
				"max": 1,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "fetch_shard_started": {
				"max": 4,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "snapshot": {
				"max": 1,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "security-token-key": {
				"max": 1,
				"queue_size": 1000,
				"type": "fixed",
				"min": 1
			  },
			  "ml_autodetect": {
				"max": 80,
				"queue_size": 80,
				"type": "fixed",
				"min": 80
			  },
			  "ml_datafeed": {
				"max": 20,
				"queue_size": 200,
				"type": "fixed",
				"min": 20
			  },
			  "fetch_shard_store": {
				"max": 4,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  }
			},
			"transport_address": "172.25.137.90:19338",
			"http": {
			  "max_content_length_in_bytes": 104857600,
			  "publish_address": "172.25.137.90:18365",
			  "bound_address": [
				"172.17.0.28:18365"
			  ]
			},
			"name": "tiebreaker-0000000005",
			"roles": [
			  "master"
			],
			"total_indexing_buffer": 62639308,
			"process": {
			  "mlockall": false,
			  "id": 530,
			  "refresh_interval_in_millis": 1000
			},
			"ip": "172.25.137.90",
			"settings": {
			  "node": {
				"attr": {
				  "region": "us-east-1",
				  "logical_availability_zone": "tiebreaker",
				  "instance_configuration": "aws.master.r4",
				  "xpack": {
					"installed": "true"
				  },
				  "availability_zone": "us-east-1e"
				},
				"ml": "false",
				"ingest": "false",
				"name": "tiebreaker-0000000005",
				"master": "true",
				"data": "false",
				"max_local_storage_nodes": "1"
			  },
			  "reindex": {
				"remote": {
				  "whitelist": [
					"*.io:*",
					"*.com:*"
				  ]
				}
			  },
			  "processors": "2",
			  "xpack": {
				"notification": {
				  "email": {
					"account": {
					  "work": {
						"email_defaults": {
						  "from": "Watcher Alert <noreply@watcheralert.found.io>"
						},
						"smtp": {
						  "host": "dockerhost",
						  "port": "10025"
						}
					  }
					}
				  }
				},
				"security": {
				  "authc": {
					"token": {
					  "enabled": "true"
					},
					"reserved_realm": {
					  "enabled": "false"
					},
					"realms": {
					  "found": {
						"type": "file",
						"order": "0"
					  },
					  "native": {
						"type": "native",
						"order": "1"
					  }
					},
					"anonymous": {
					  "username": "anonymous",
					  "authz_exception": "false",
					  "roles": "anonymous"
					}
				  },
				  "enabled": "true",
				  "http": {
					"ssl": {
					  "enabled": "true"
					}
				  },
				  "transport": {
					"ssl": {
					  "enabled": "true"
					}
				  }
				},
				"monitoring": {
				  "collection": {
					"enabled": "false"
				  },
				  "history": {
					"duration": "3d"
				  }
				},
				"license": {
				  "self_generated": {
					"type": "trial"
				  }
				},
				"ml": {
				  "enabled": "true"
				}
			  },
			  "script": {
				"allowed_types": "stored,inline"
			  },
			  "s3": {
				"client": {
				  "default": {
					"endpoint": "s3.amazonaws.com"
				  }
				}
			  },
			  "cluster": {
				"indices": {
				  "close": {
					"enable": "false"
				  }
				},
				"name": "af932d24216a4dd69ba47d2fd3214796",
				"routing": {
				  "allocation": {
					"disk": {
					  "threshold_enabled": "false"
					},
					"awareness": {
					  "attributes": "region,availability_zone,logical_availability_zone"
					}
				  }
				}
			  },
			  "client": {
				"type": "node"
			  },
			  "action": {
				"auto_create_index": "true",
				"destructive_requires_name": "false"
			  },
			  "pidfile": "/app/es.pid",
			  "discovery": {
				"zen": {
				  "minimum_master_nodes": "4",
				  "ping": {
					"unicast": {
					  "hosts": "172.25.28.128:19760,172.25.44.68:19866,172.25.133.112:19625,172.25.85.111:19673,172.25.98.100:19890"
					}
				  },
				  "hosts_provider": "found"
				}
			  }
			},
			"modules": [
			  {
				"has_native_controller": false,
				"description": "Adds aggregations whose input are a list of numeric fields and output includes a matrix.",
				"java_version": "1.8",
				"classname": "org.elasticsearch.search.aggregations.matrix.MatrixAggregationPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "aggs-matrix-stats"
			  },
			  {
				"has_native_controller": false,
				"description": "Adds \"built in\" analyzers to Elasticsearch.",
				"java_version": "1.8",
				"classname": "org.elasticsearch.analysis.common.CommonAnalysisPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "analysis-common"
			  },
			  {
				"has_native_controller": false,
				"description": "Module for ingest processors that do not require additional security permissions or have large dependencies and resources",
				"java_version": "1.8",
				"classname": "org.elasticsearch.ingest.common.IngestCommonPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "lang-painless"
				],
				"name": "ingest-common"
			  },
			  {
				"has_native_controller": false,
				"description": "Lucene expressions integration for Elasticsearch",
				"java_version": "1.8",
				"classname": "org.elasticsearch.script.expression.ExpressionPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "lang-expression"
			  },
			  {
				"has_native_controller": false,
				"description": "Mustache scripting integration for Elasticsearch",
				"java_version": "1.8",
				"classname": "org.elasticsearch.script.mustache.MustachePlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "lang-mustache"
			  },
			  {
				"has_native_controller": false,
				"description": "An easy, safe and fast scripting language for Elasticsearch",
				"java_version": "1.8",
				"classname": "org.elasticsearch.painless.PainlessPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "lang-painless"
			  },
			  {
				"has_native_controller": false,
				"description": "Adds advanced field mappers",
				"java_version": "1.8",
				"classname": "org.elasticsearch.index.mapper.MapperExtrasPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "mapper-extras"
			  },
			  {
				"has_native_controller": false,
				"description": "This module adds the support parent-child queries and aggregations",
				"java_version": "1.8",
				"classname": "org.elasticsearch.join.ParentJoinPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "parent-join"
			  },
			  {
				"has_native_controller": false,
				"description": "Percolator module adds capability to index queries and query these queries by specifying documents",
				"java_version": "1.8",
				"classname": "org.elasticsearch.percolator.PercolatorPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "percolator"
			  },
			  {
				"has_native_controller": false,
				"description": "The Rank Eval module adds APIs to evaluate ranking quality.",
				"java_version": "1.8",
				"classname": "org.elasticsearch.index.rankeval.RankEvalPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "rank-eval"
			  },
			  {
				"has_native_controller": false,
				"description": "The Reindex module adds APIs to reindex from one index to another or update documents in place.",
				"java_version": "1.8",
				"classname": "org.elasticsearch.index.reindex.ReindexPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "reindex"
			  },
			  {
				"has_native_controller": false,
				"description": "Module for URL repository",
				"java_version": "1.8",
				"classname": "org.elasticsearch.plugin.repository.url.URLRepositoryPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "repository-url"
			  },
			  {
				"has_native_controller": false,
				"description": "Netty 4 based transport implementation",
				"java_version": "1.8",
				"classname": "org.elasticsearch.transport.Netty4Plugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "transport-netty4"
			  },
			  {
				"has_native_controller": false,
				"description": "Tribe module",
				"java_version": "1.8",
				"classname": "org.elasticsearch.tribe.TribePlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "tribe"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Core",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.core.XPackPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "x-pack-core"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Deprecation",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.deprecation.Deprecation",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-deprecation"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Graph",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.graph.Graph",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-graph"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Logstash",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.logstash.Logstash",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-logstash"
			  },
			  {
				"has_native_controller": true,
				"description": "Elasticsearch Expanded Pack Plugin - Machine Learning",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.ml.MachineLearning",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-ml"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Monitoring",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.monitoring.Monitoring",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-monitoring"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Rollup",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.rollup.Rollup",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-rollup"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Security",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.security.Security",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-security"
			  },
			  {
				"has_native_controller": false,
				"description": "The Elasticsearch plugin that powers SQL for Elasticsearch",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.sql.plugin.SqlPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core",
				  "lang-painless"
				],
				"name": "x-pack-sql"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Upgrade",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.upgrade.Upgrade",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-upgrade"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Watcher",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.watcher.Watcher",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-watcher"
			  }
			],
			"ingest": {
			  "processors": [
				{
				  "type": "append"
				},
				{
				  "type": "bytes"
				},
				{
				  "type": "convert"
				},
				{
				  "type": "date"
				},
				{
				  "type": "date_index_name"
				},
				{
				  "type": "dot_expander"
				},
				{
				  "type": "fail"
				},
				{
				  "type": "foreach"
				},
				{
				  "type": "geoip"
				},
				{
				  "type": "grok"
				},
				{
				  "type": "gsub"
				},
				{
				  "type": "join"
				},
				{
				  "type": "json"
				},
				{
				  "type": "kv"
				},
				{
				  "type": "lowercase"
				},
				{
				  "type": "remove"
				},
				{
				  "type": "rename"
				},
				{
				  "type": "script"
				},
				{
				  "type": "set"
				},
				{
				  "type": "set_security_user"
				},
				{
				  "type": "sort"
				},
				{
				  "type": "split"
				},
				{
				  "type": "trim"
				},
				{
				  "type": "uppercase"
				},
				{
				  "type": "urldecode"
				},
				{
				  "type": "user_agent"
				}
			  ]
			},
			"host": "172.25.137.90",
			"version": "6.4.1",
			"jvm": {
			  "vm_name": "Java HotSpot(TM) 64-Bit Server VM",
			  "vm_version": "25.144-b01",
			  "mem": {
				"non_heap_max_in_bytes": 0,
				"heap_init_in_bytes": 643825664,
				"heap_max_in_bytes": 626393088,
				"direct_max_in_bytes": 626393088,
				"non_heap_init_in_bytes": 2555904
			  },
			  "gc_collectors": [
				"ParNew",
				"ConcurrentMarkSweep"
			  ],
			  "using_compressed_ordinary_object_pointers": "true",
			  "pid": 530,
			  "input_arguments": [
				"-XX:+UseConcMarkSweepGC",
				"-XX:CMSInitiatingOccupancyFraction=75",
				"-XX:+UseCMSInitiatingOccupancyOnly",
				"-XX:+AlwaysPreTouch",
				"-Xss1m",
				"-Djava.awt.headless=true",
				"-Dfile.encoding=UTF-8",
				"-Djna.nosys=true",
				"-XX:-OmitStackTraceInFastThrow",
				"-Dio.netty.noUnsafe=true",
				"-Dio.netty.noKeySetOptimization=true",
				"-Dio.netty.recycler.maxCapacityPerThread=0",
				"-Dlog4j.shutdownHookEnabled=false",
				"-Dlog4j2.disable.jmx=true",
				"-Djava.io.tmpdir=/tmp/elasticsearch.kjxtkeVG",
				"-XX:HeapDumpPath=data",
				"-XX:ErrorFile=logs/hs_err_pid%p.log",
				"-XX:+PrintGCDetails",
				"-XX:+PrintGCDateStamps",
				"-XX:+PrintTenuringDistribution",
				"-XX:+PrintGCApplicationStoppedTime",
				"-Xloggc:/app/logs/gc.log",
				"-XX:+UseGCLogFileRotation",
				"-XX:NumberOfGCLogFiles=2",
				"-XX:GCLogFileSize=8m",
				"-Des.allow_insecure_settings=true",
				"-XX:ParallelGCThreads=2",
				"-XX:ConcGCThreads=1",
				"-Xms614M",
				"-Xmx614M",
				"-Dio.netty.allocator.type=unpooled",
				"-Djava.nio.file.spi.DefaultFileSystemProvider=co.elastic.cloud.quotaawarefs.QuotaAwareFileSystemProvider",
				"-Dcurator-log-only-first-connection-issue-as-error-level=true",
				"-Dio.netty.recycler.maxCapacityPerThread=0",
				"-Djava.security.policy=file:///app/config/gelf.policy",
				"-Des.cgroups.hierarchy.override=/",
				"-Des.geoip.load_db_on_heap=true",
				"-Des.path.home=/elasticsearch",
				"-Des.path.conf=/app/config",
				"-Des.distribution.flavor=default",
				"-Des.distribution.type=tar"
			  ],
			  "version": "1.8.0_144",
			  "vm_vendor": "Oracle Corporation",
			  "memory_pools": [
				"Code Cache",
				"Metaspace",
				"Compressed Class Space",
				"Par Eden Space",
				"Par Survivor Space",
				"CMS Old Gen"
			  ],
			  "start_time_in_millis": 1543481391195
			},
			"build_flavor": "default",
			"build_hash": "e36acdb",
			"attributes": {
			  "instance_configuration": "aws.master.r4",
			  "region": "us-east-1",
			  "logical_availability_zone": "tiebreaker",
			  "xpack.installed": "true",
			  "availability_zone": "us-east-1e"
			},
			"os": {
			  "name": "Linux",
			  "allocated_processors": 2,
			  "version": "4.4.0-1048-aws",
			  "arch": "amd64",
			  "refresh_interval_in_millis": 1000,
			  "available_processors": 4
			},
			"build_type": "tar",
			"transport": {
			  "publish_address": "172.25.137.90:19338",
			  "bound_address": [
				"172.17.0.28:19338"
			  ],
			  "profiles": {
				"client": {
				  "publish_address": "172.17.0.28:20060",
				  "bound_address": [
					"172.17.0.28:20060"
				  ]
				}
			  }
			}
		  },
		  "DDySQDLCSHWvcFeyI2UfMA": {
			"plugins": [
			  {
				"has_native_controller": false,
				"description": "Elasticsearch plugin for Found",
				"java_version": "1.8",
				"classname": "org.elasticsearch.plugin.found.FoundPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "found-elasticsearch"
			  },
			  {
				"has_native_controller": false,
				"description": "Ingest processor that uses looksup geo data based on ip adresses using the Maxmind geo database",
				"java_version": "1.8",
				"classname": "org.elasticsearch.ingest.geoip.IngestGeoIpPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "ingest-geoip"
			  },
			  {
				"has_native_controller": false,
				"description": "Ingest processor that extracts information from a user agent",
				"java_version": "1.8",
				"classname": "org.elasticsearch.ingest.useragent.IngestUserAgentPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "ingest-user-agent"
			  },
			  {
				"has_native_controller": false,
				"description": "The S3 repository plugin adds S3 repositories",
				"java_version": "1.8",
				"classname": "org.elasticsearch.repositories.s3.S3RepositoryPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "repository-s3"
			  }
			],
			"thread_pool": {
			  "ml_utility": {
				"max": 80,
				"queue_size": 500,
				"type": "fixed",
				"min": 80
			  },
			  "index": {
				"max": 2,
				"queue_size": 200,
				"type": "fixed",
				"min": 2
			  },
			  "search": {
				"max": 4,
				"queue_size": 1000,
				"type": "fixed_auto_queue_size",
				"min": 4
			  },
			  "force_merge": {
				"max": 1,
				"queue_size": -1,
				"type": "fixed",
				"min": 1
			  },
			  "get": {
				"max": 2,
				"queue_size": 1000,
				"type": "fixed",
				"min": 2
			  },
			  "generic": {
				"max": 128,
				"queue_size": -1,
				"keep_alive": "30s",
				"type": "scaling",
				"min": 4
			  },
			  "analyze": {
				"max": 1,
				"queue_size": 16,
				"type": "fixed",
				"min": 1
			  },
			  "write": {
				"max": 2,
				"queue_size": 200,
				"type": "fixed",
				"min": 2
			  },
			  "refresh": {
				"max": 1,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "watcher": {
				"max": 10,
				"queue_size": 1000,
				"type": "fixed",
				"min": 10
			  },
			  "listener": {
				"max": 1,
				"queue_size": -1,
				"type": "fixed",
				"min": 1
			  },
			  "rollup_indexing": {
				"max": 4,
				"queue_size": 4,
				"type": "fixed",
				"min": 4
			  },
			  "management": {
				"max": 5,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "warmer": {
				"max": 1,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "flush": {
				"max": 1,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "fetch_shard_started": {
				"max": 4,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "snapshot": {
				"max": 1,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  },
			  "security-token-key": {
				"max": 1,
				"queue_size": 1000,
				"type": "fixed",
				"min": 1
			  },
			  "ml_autodetect": {
				"max": 80,
				"queue_size": 80,
				"type": "fixed",
				"min": 80
			  },
			  "ml_datafeed": {
				"max": 20,
				"queue_size": 200,
				"type": "fixed",
				"min": 20
			  },
			  "fetch_shard_store": {
				"max": 4,
				"queue_size": -1,
				"keep_alive": "5m",
				"type": "scaling",
				"min": 1
			  }
			},
			"transport_address": "172.25.133.112:19447",
			"http": {
			  "max_content_length_in_bytes": 104857600,
			  "publish_address": "172.25.133.112:18269",
			  "bound_address": [
				"172.17.0.29:18269"
			  ]
			},
			"name": "instance-0000000006",
			"roles": [
			  "master",
			  "data",
			  "ingest"
			],
			"total_indexing_buffer": 213005107,
			"process": {
			  "mlockall": false,
			  "id": 915,
			  "refresh_interval_in_millis": 1000
			},
			"ip": "172.25.133.112",
			"settings": {
			  "node": {
				"attr": {
				  "region": "us-east-1",
				  "logical_availability_zone": "zone-0",
				  "instance_configuration": "aws.data.highio.i3",
				  "xpack": {
					"installed": "true"
				  },
				  "availability_zone": "us-east-1e"
				},
				"ml": "false",
				"ingest": "true",
				"name": "instance-0000000006",
				"master": "true",
				"data": "true",
				"max_local_storage_nodes": "1"
			  },
			  "reindex": {
				"remote": {
				  "whitelist": [
					"*.io:*",
					"*.com:*"
				  ]
				}
			  },
			  "processors": "2",
			  "xpack": {
				"notification": {
				  "email": {
					"account": {
					  "work": {
						"email_defaults": {
						  "from": "Watcher Alert <noreply@watcheralert.found.io>"
						},
						"smtp": {
						  "host": "dockerhost",
						  "port": "10025"
						}
					  }
					}
				  }
				},
				"security": {
				  "authc": {
					"token": {
					  "enabled": "true"
					},
					"reserved_realm": {
					  "enabled": "false"
					},
					"realms": {
					  "found": {
						"type": "file",
						"order": "0"
					  },
					  "native": {
						"type": "native",
						"order": "1"
					  }
					},
					"anonymous": {
					  "username": "anonymous",
					  "authz_exception": "false",
					  "roles": "anonymous"
					}
				  },
				  "enabled": "true",
				  "http": {
					"ssl": {
					  "enabled": "true"
					}
				  },
				  "transport": {
					"ssl": {
					  "enabled": "true"
					}
				  }
				},
				"monitoring": {
				  "collection": {
					"enabled": "false"
				  },
				  "history": {
					"duration": "3d"
				  }
				},
				"license": {
				  "self_generated": {
					"type": "trial"
				  }
				},
				"ml": {
				  "enabled": "true"
				}
			  },
			  "script": {
				"allowed_types": "stored,inline"
			  },
			  "s3": {
				"client": {
				  "default": {
					"endpoint": "s3.amazonaws.com"
				  }
				}
			  },
			  "cluster": {
				"indices": {
				  "close": {
					"enable": "false"
				  }
				},
				"name": "af932d24216a4dd69ba47d2fd3214796",
				"routing": {
				  "allocation": {
					"disk": {
					  "threshold_enabled": "false"
					},
					"awareness": {
					  "attributes": "region,availability_zone,logical_availability_zone"
					}
				  }
				}
			  },
			  "client": {
				"type": "node"
			  },
			  "action": {
				"auto_create_index": "true",
				"destructive_requires_name": "false"
			  },
			  "pidfile": "/app/es.pid",
			  "discovery": {
				"zen": {
				  "minimum_master_nodes": "3",
				  "ping": {
					"unicast": {
					  "hosts": "172.25.137.90:19338,172.25.85.111:19673,172.25.44.68:19866"
					}
				  },
				  "hosts_provider": "found"
				}
			  }
			},
			"modules": [
			  {
				"has_native_controller": false,
				"description": "Adds aggregations whose input are a list of numeric fields and output includes a matrix.",
				"java_version": "1.8",
				"classname": "org.elasticsearch.search.aggregations.matrix.MatrixAggregationPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "aggs-matrix-stats"
			  },
			  {
				"has_native_controller": false,
				"description": "Adds \"built in\" analyzers to Elasticsearch.",
				"java_version": "1.8",
				"classname": "org.elasticsearch.analysis.common.CommonAnalysisPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "analysis-common"
			  },
			  {
				"has_native_controller": false,
				"description": "Module for ingest processors that do not require additional security permissions or have large dependencies and resources",
				"java_version": "1.8",
				"classname": "org.elasticsearch.ingest.common.IngestCommonPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "lang-painless"
				],
				"name": "ingest-common"
			  },
			  {
				"has_native_controller": false,
				"description": "Lucene expressions integration for Elasticsearch",
				"java_version": "1.8",
				"classname": "org.elasticsearch.script.expression.ExpressionPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "lang-expression"
			  },
			  {
				"has_native_controller": false,
				"description": "Mustache scripting integration for Elasticsearch",
				"java_version": "1.8",
				"classname": "org.elasticsearch.script.mustache.MustachePlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "lang-mustache"
			  },
			  {
				"has_native_controller": false,
				"description": "An easy, safe and fast scripting language for Elasticsearch",
				"java_version": "1.8",
				"classname": "org.elasticsearch.painless.PainlessPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "lang-painless"
			  },
			  {
				"has_native_controller": false,
				"description": "Adds advanced field mappers",
				"java_version": "1.8",
				"classname": "org.elasticsearch.index.mapper.MapperExtrasPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "mapper-extras"
			  },
			  {
				"has_native_controller": false,
				"description": "This module adds the support parent-child queries and aggregations",
				"java_version": "1.8",
				"classname": "org.elasticsearch.join.ParentJoinPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "parent-join"
			  },
			  {
				"has_native_controller": false,
				"description": "Percolator module adds capability to index queries and query these queries by specifying documents",
				"java_version": "1.8",
				"classname": "org.elasticsearch.percolator.PercolatorPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "percolator"
			  },
			  {
				"has_native_controller": false,
				"description": "The Rank Eval module adds APIs to evaluate ranking quality.",
				"java_version": "1.8",
				"classname": "org.elasticsearch.index.rankeval.RankEvalPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "rank-eval"
			  },
			  {
				"has_native_controller": false,
				"description": "The Reindex module adds APIs to reindex from one index to another or update documents in place.",
				"java_version": "1.8",
				"classname": "org.elasticsearch.index.reindex.ReindexPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "reindex"
			  },
			  {
				"has_native_controller": false,
				"description": "Module for URL repository",
				"java_version": "1.8",
				"classname": "org.elasticsearch.plugin.repository.url.URLRepositoryPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "repository-url"
			  },
			  {
				"has_native_controller": false,
				"description": "Netty 4 based transport implementation",
				"java_version": "1.8",
				"classname": "org.elasticsearch.transport.Netty4Plugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "transport-netty4"
			  },
			  {
				"has_native_controller": false,
				"description": "Tribe module",
				"java_version": "1.8",
				"classname": "org.elasticsearch.tribe.TribePlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "tribe"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Core",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.core.XPackPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [],
				"name": "x-pack-core"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Deprecation",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.deprecation.Deprecation",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-deprecation"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Graph",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.graph.Graph",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-graph"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Logstash",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.logstash.Logstash",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-logstash"
			  },
			  {
				"has_native_controller": true,
				"description": "Elasticsearch Expanded Pack Plugin - Machine Learning",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.ml.MachineLearning",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-ml"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Monitoring",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.monitoring.Monitoring",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-monitoring"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Rollup",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.rollup.Rollup",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-rollup"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Security",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.security.Security",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-security"
			  },
			  {
				"has_native_controller": false,
				"description": "The Elasticsearch plugin that powers SQL for Elasticsearch",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.sql.plugin.SqlPlugin",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core",
				  "lang-painless"
				],
				"name": "x-pack-sql"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Upgrade",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.upgrade.Upgrade",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-upgrade"
			  },
			  {
				"has_native_controller": false,
				"description": "Elasticsearch Expanded Pack Plugin - Watcher",
				"java_version": "1.8",
				"classname": "org.elasticsearch.xpack.watcher.Watcher",
				"version": "6.4.1",
				"elasticsearch_version": "6.4.1",
				"extended_plugins": [
				  "x-pack-core"
				],
				"name": "x-pack-watcher"
			  }
			],
			"ingest": {
			  "processors": [
				{
				  "type": "append"
				},
				{
				  "type": "bytes"
				},
				{
				  "type": "convert"
				},
				{
				  "type": "date"
				},
				{
				  "type": "date_index_name"
				},
				{
				  "type": "dot_expander"
				},
				{
				  "type": "fail"
				},
				{
				  "type": "foreach"
				},
				{
				  "type": "geoip"
				},
				{
				  "type": "grok"
				},
				{
				  "type": "gsub"
				},
				{
				  "type": "join"
				},
				{
				  "type": "json"
				},
				{
				  "type": "kv"
				},
				{
				  "type": "lowercase"
				},
				{
				  "type": "remove"
				},
				{
				  "type": "rename"
				},
				{
				  "type": "script"
				},
				{
				  "type": "set"
				},
				{
				  "type": "set_security_user"
				},
				{
				  "type": "sort"
				},
				{
				  "type": "split"
				},
				{
				  "type": "trim"
				},
				{
				  "type": "uppercase"
				},
				{
				  "type": "urldecode"
				},
				{
				  "type": "user_agent"
				}
			  ]
			},
			"host": "172.25.133.112",
			"version": "6.4.1",
			"jvm": {
			  "vm_name": "Java HotSpot(TM) 64-Bit Server VM",
			  "vm_version": "25.144-b01",
			  "mem": {
				"non_heap_max_in_bytes": 0,
				"heap_init_in_bytes": 2147483648,
				"heap_max_in_bytes": 2130051072,
				"direct_max_in_bytes": 2130051072,
				"non_heap_init_in_bytes": 2555904
			  },
			  "gc_collectors": [
				"ParNew",
				"ConcurrentMarkSweep"
			  ],
			  "using_compressed_ordinary_object_pointers": "true",
			  "pid": 915,
			  "input_arguments": [
				"-XX:+UseConcMarkSweepGC",
				"-XX:CMSInitiatingOccupancyFraction=75",
				"-XX:+UseCMSInitiatingOccupancyOnly",
				"-XX:+AlwaysPreTouch",
				"-Xss1m",
				"-Djava.awt.headless=true",
				"-Dfile.encoding=UTF-8",
				"-Djna.nosys=true",
				"-XX:-OmitStackTraceInFastThrow",
				"-Dio.netty.noUnsafe=true",
				"-Dio.netty.noKeySetOptimization=true",
				"-Dio.netty.recycler.maxCapacityPerThread=0",
				"-Dlog4j.shutdownHookEnabled=false",
				"-Dlog4j2.disable.jmx=true",
				"-Djava.io.tmpdir=/tmp/elasticsearch.98UPLuoC",
				"-XX:HeapDumpPath=data",
				"-XX:ErrorFile=logs/hs_err_pid%p.log",
				"-XX:+PrintGCDetails",
				"-XX:+PrintGCDateStamps",
				"-XX:+PrintTenuringDistribution",
				"-XX:+PrintGCApplicationStoppedTime",
				"-Xloggc:/app/logs/gc.log",
				"-XX:+UseGCLogFileRotation",
				"-XX:NumberOfGCLogFiles=2",
				"-XX:GCLogFileSize=8m",
				"-Des.allow_insecure_settings=true",
				"-XX:ParallelGCThreads=2",
				"-XX:ConcGCThreads=1",
				"-Xms2048M",
				"-Xmx2048M",
				"-Djava.nio.file.spi.DefaultFileSystemProvider=co.elastic.cloud.quotaawarefs.QuotaAwareFileSystemProvider",
				"-Dcurator-log-only-first-connection-issue-as-error-level=true",
				"-Dio.netty.recycler.maxCapacityPerThread=0",
				"-Djava.security.policy=file:///app/config/gelf.policy",
				"-Des.cgroups.hierarchy.override=/",
				"-Des.geoip.load_db_on_heap=true",
				"-Des.path.home=/elasticsearch",
				"-Des.path.conf=/app/config",
				"-Des.distribution.flavor=default",
				"-Des.distribution.type=tar"
			  ],
			  "version": "1.8.0_144",
			  "vm_vendor": "Oracle Corporation",
			  "memory_pools": [
				"Code Cache",
				"Metaspace",
				"Compressed Class Space",
				"Par Eden Space",
				"Par Survivor Space",
				"CMS Old Gen"
			  ],
			  "start_time_in_millis": 1543481961665
			},
			"build_flavor": "default",
			"build_hash": "e36acdb",
			"attributes": {
			  "instance_configuration": "aws.data.highio.i3",
			  "region": "us-east-1",
			  "logical_availability_zone": "zone-0",
			  "xpack.installed": "true",
			  "availability_zone": "us-east-1e"
			},
			"os": {
			  "name": "Linux",
			  "allocated_processors": 2,
			  "version": "4.4.0-1048-aws",
			  "arch": "amd64",
			  "refresh_interval_in_millis": 1000,
			  "available_processors": 32
			},
			"build_type": "tar",
			"transport": {
			  "publish_address": "172.25.133.112:19447",
			  "bound_address": [
				"172.17.0.29:19447"
			  ],
			  "profiles": {
				"client": {
				  "publish_address": "172.17.0.29:20043",
				  "bound_address": [
					"172.17.0.29:20043"
				  ]
				}
			  }
			}
		  }
		}
	  }
	  `
)
