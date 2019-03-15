# Lightweight Process Manager

Goals:
- Control the Elasticsearch process (stop/start)
- Fold the keystore-updater & the cert initializer
- Report the keystore-updater health
- Be extremely conservative with resource usage

# Configuration

Flags to declare the process to manage:

    --name es
    --cmd /usr/local/bin/docker-entrypoint.sh

Env vars to configure the keystore-updater:

	SOURCE_DIR=/volumes/secrets
	KEYSTORE_BINARY=/usr/share/elasticsearch/bin/elasticsearch-keystore
	KEYSTORE_PATH=/usr/share/elasticsearch/config/elasticsearch.keystore
	RELOAD_CREDENTIALS=false
	USERNAME=
	PASSWORD=
	PASSWORD_FILE=
	CERTIFICATES_PATH=/volume/node-certs
	ENDPOINT=https://127.0.0.1:9200

Flags to configure the cert initializer:

	--port
	--private-key-path
	--cert-path
	--csr-path

# HTTP API

Exposes the control of the Elasticsearch process over HTTP.

```
/health
/es/start
/es/stop
/es/restart
/es/kill
/es/status
```

# FAQ

#### Blocking/non-blocking start/stop?

- Non-blocking start
- Blocking stop with a timeout (5s)

#### Stop behaviour?

Kill in 2 phases: 
- 1: soft kill (SIGTERM)
- 2: hard kill (SIGKILL)
    - immediately after the soft kill success, to try to kill resilient child processes
    - after a timeout, if the kill soft fails

#### Progress group usage to avoid zombies processes?

The process is started with a dedicated group to forward signals to the main process and all children:

`cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`.

When killing the process, the signal is sent to the negation of the group number:

`syscall.Kill(-(pgid), killSignal)`

#### Reap zombies processes?

How does it work?

It blocks waiting for child processes to exit. It's done by listening `SIGCHLD` signals and 
attempt to reap abandoned child processes by calling:

 `unix.Wait4(-1, &status, unix.WNOHANG, nil)`.

This can steal return values from uses of packages like Go's exec.

Some examples:
- https://github.com/hashicorp/go-reap/blob/master/reap_unix.go
- https://github.com/aerokube/init/blob/master/init.go#L50 
- https://github.com/ramr/go-reaper/blob/master/reaper.go#L40
- https://github.com/pablo-ruth/go-init/blob/master/main.go#L94
