# Lightweight Process Manager

Goals:
- Control the Elasticsearch process (start/stop/kill)
- Run the keystore-updater

## Configuration

Env vars to configure the process-manager:

```bash
PM_PROC_NAME=es
PM_PROC_CMD=/usr/local/bin/docker-entrypoint.sh
PM_REAPER=true
PM_HTTP_PORT=8080
PM_TLS=false
PM_CERT_PATH=
PM_KEY_PATH=
PM_KEYSTORE_UPDATER=true
PM_EXP_VARS=false
PM_PROFILER=false

```

Env vars to configure the keystore-updater:

```bash
KEYSTORE_SOURCE_DIR=/volumes/secrets
KEYSTORE_BINARY=/usr/share/elasticsearch/bin/elasticsearch-keystore
KEYSTORE_PATH=/usr/share/elasticsearch/config/elasticsearch.keystore
KEYSTORE_RELOAD_CREDENTIALS=false
KEYSTORE_ES_USERNAME=
KEYSTORE_ES_PASSWORD=
KEYSTORE_ES_PASSWORD_FILE=
KEYSTORE_ES_CA_CERTS_PATH=/volume/http-certs
KEYSTORE_ES_ENDPOINT=https://127.0.0.1:9200
KEYSTORE_ES_VERSION=
```

## HTTP API

Exposes the control of the Elasticsearch process over HTTP or HTTPS.

```
GET     /health                         =>  200 || 500
POST    /es/start                       =>  200, started  || 500
POST    /es/stop                        =>  202, stopping || 200, stopped || 500
POST    /es/kill                        =>  202, killing  || 200, killed  || 500
GET     /es/status                      =>  200 || 500
GET     /keystore/status                =>  200 || 500
```

## FAQ

#### Blocking/non-blocking start/stop?

The start and stop endpoints are non-blocking and idempotent.

#### Stop vs kill?

The stop endpoint does a soft kill (SIGTERM) while the kill endpoint does a hard kill (SIGKILL).

#### What happens when the process manager is terminated or killed?

The Elasticsearch process does the same.
All signals received by the process manager are forwarded to the Elasticsearch process. 

### What happens when the Elasticsearch process dies in background?

The process manager exits.

#### Process group usage to avoid zombies processes?

The process is started with a dedicated group to forward signals to the main process and all children:

`cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`.

When killing the process, the signal is sent to the negation of the group number:

`syscall.Kill(-(pgid), killSignal)`

#### Reap zombies processes

A Zombies processes reap can be enabled at the start of the process manager.

How does it work?

It blocks waiting for child processes to exit. It's done by listening `SIGCHLD` signals and 
attempting to reap abandoned child processes by calling:

`unix.Wait4(-1, &status, unix.WNOHANG, nil)`.

This can steal return values from uses of packages like Go's exec.

