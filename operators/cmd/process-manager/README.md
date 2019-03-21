# Lightweight Process Manager

Goals:
- Control the Elasticsearch process (stop/start)
- Run the keystore-updater

## Configuration

Env vars to configure the process-manager:

	PM_PROC_NAME=es
	PM_PROC_CMD=/usr/local/bin/docker-entrypoint.sh
	PM_REAPER=false
	PM_TLS=false
	
Env vars to configure the keystore-updater:

	KEYSTORE_SOURCE_DIR=/volumes/secrets
	KEYSTORE_BINARY=/usr/share/elasticsearch/bin/elasticsearch-keystore
	KEYSTORE_PATH=/usr/share/elasticsearch/config/elasticsearch.keystore
	KEYSTORE_RELOAD_CREDENTIALS=false
	KEYSTORE_ES_USERNAME=
	KEYSTORE_ES_PASSWORD=
	KEYSTORE_ES_PASSWORD_FILE=
	KEYSTORE_ES_CA_CERTS_PATH=/volume/node-certs
	KEYSTORE_ES_ENDPOINT=https://127.0.0.1:9200
	KEYSTORE_ES_VERSION=

## HTTP API

Exposes the control of the Elasticsearch process over HTTP or HTTPS.

```
GET     /health                         =>  200 || 500
POST    /es/start                       =>  202, starting || 200, started || 500
POST    /es/stop?hard=true&timeout=10   =>  202, stopping || 200, stopped || 500
GET     /es/status                      =>  200 || 500
GET     /keystore/status                =>  200 || 500
```

## FAQ

#### Blocking/non-blocking start/stop?

The start and stop endpoints are non-blocking and idempotent.

#### Stop behaviour?

By default, the stop is a soft kill (SIGTERM).

A hard kill can be forced (`/es/stop?force=true`).

An overridable timeout is configured to kill hard (SIGKILL) if the process has not terminated fast enough `/es/stop?timeout=10`.

#### Process group usage to avoid zombies processes?

The process is started with a dedicated group to forward signals to the main process and all children:

`cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`.

When killing the process, the signal is sent to the negation of the group number:

`syscall.Kill(-(pgid), killSignal)`

#### Reap zombies processes?

A Zombies processes repear can be started.

How does it work?

It blocks waiting for child processes to exit. It's done by listening `SIGCHLD` signals and 
attempt to reap abandoned child processes by calling:

 `unix.Wait4(-1, &status, unix.WNOHANG, nil)`.

This can steal return values from uses of packages like Go's exec.

Heavily inspired from https://github.com/hashicorp/go-reap/blob/master/reap_unix.go.


