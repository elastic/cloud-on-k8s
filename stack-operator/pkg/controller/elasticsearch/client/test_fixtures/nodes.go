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
				"transport_address": "172.25.98.100:19850",
				"name": "instance-0000000007",
				"roles": [
					"master",
					"data",
					"ingest"
				],
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
				"ip": "172.25.98.100",
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
				"build_type": "tar"
			},
			"EwsDTq-KSny1gbUcH77nxA": {
				"transport_address": "172.25.137.90:19338",
				"name": "tiebreaker-0000000005",
				"roles": [
					"master"
				],
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
				"ip": "172.25.137.90",
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
				"build_type": "tar"
			},
			"DDySQDLCSHWvcFeyI2UfMA": {
				"transport_address": "172.25.133.112:19447",
				"name": "instance-0000000006",
				"roles": [
					"master",
					"data",
					"ingest"
				],
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
				"ip": "172.25.133.112",
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
				"build_type": "tar"
			}
		}
	}
	  `
)
