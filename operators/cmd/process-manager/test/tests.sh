#!/bin/bash -u

logs=y

pm::start() {
	docker run -d \
		--name pm \
		-p 8080:8080 \
		-e PROC_NAME=es \
		-e PROC_CMD="$@" \
		pm
}

pm::stop() {
	docker stop pm > /dev/null
	docker rm pm > /dev/null
}

pm::api() {
	curl -s "127.0.0.1:8080/elasticsearch/$1" \
	    -w ', status: %{http_code}\n'
}

startstop() {
    declare cmd=$1 stop=$2 expected=$3

    pm::start "$cmd"
    [[ "logs" != "" ]] && (docker logs pm -f &)

    pm::api start
    sleep 3
    pm::api $stop
    sleep 3
    pm::api $stop

    check "$cmd" $stop $expected

    pm::stop
    [[ "logs" != "" ]] && wait
}

check() {
    declare cmd=$1 stop=$2 expected=$3
    count=$(ps -eo pid,ppid,pgid,cmd | grep -v grep | grep -cE "(script|zb)")
    if [[ "$expected" == n && $count -le 0 ]]; then
        echo "failed: cmd: cmd, stop: $stop, is: $count, expected: $expected"
    fi
    if [[ "$expected" == 0 && $count -ne 0 ]]; then
        echo "failed: cmd: $cmd, stop: $stop, is: $count, expected: $expected"
    fi

    echo "ok: cmd: $cmd is: $count"
}

docker rm -f pm > /dev/null

startstop "ex/script" "stop" 0
startstop "ex/script z" "stop" 0
startstop "ex/script z e" "stop" n
startstop "ex/script z e" "stop?timeout=2" 0