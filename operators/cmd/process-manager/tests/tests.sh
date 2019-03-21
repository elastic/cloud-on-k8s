#!/bin/bash -u

LOG=${LOG:-}
PORT=${PORT:-8081}

run() {
    declare cmd=$1 stop=$2 expected=$3

    echo "> $@ "

    # Start
    pm::start "$cmd"
    [[ "$LOG" != "" ]] && (docker logs pm -f &)

    # Wait a bit
    sleep 3
    pm::api start

    # Stop
    pm::api "$stop"
    
    sleep 3
    pm::api "$stop"

    check "$cmd" "$stop" "$expected"

    pm::stop
    [[ "$LOG" != "" ]] && wait
}

pm::start() {
	docker run -d \
		--name pm \
		-p "$PORT:8080" \
		-e PM_PROC_NAME=es \
		-e PM_PROC_CMD="$@" \
		pm > /dev/null
}

pm::stop() {
	docker stop pm > /dev/null
	docker rm pm > /dev/null
}

pm::api() {
	curl -s "127.0.0.1:$PORT/es/$1" \
	    -w ', status: %{http_code}\n'
}

check() {
    local green="\e[32m"
      local red="\e[31m"
      local reset="\e[39m"
  
    declare cmd=$1 stop=$2 expected=$3
    count=$(ps -eo pid,ppid,pgid,cmd | grep -v grep | grep -cE "(script|zb)")
    if [[ "$expected" == n && $count -le 0 ]]; then
        echo -e "${red}failed${reset}: $cmd, with $stop, processes: $count, expected: $expected"
    elif [[ "$expected" == 0 && $count -ne 0 ]]; then
        echo -e "${red}failed${reset}: $cmd, with $stop, processes: $count, expected: $expected"
    else
        echo -e "${green}ok${reset}: cmd: $cmd processes: $count"
    fi
}

docker rm -f pm &> /dev/null

main() {
    run "bin/script"     "stop" 0
    run "bin/script z"   "stop" 0
    run "bin/script z e" "stop" n
    run "bin/script z e" "stop?force=true" 0
    run "bin/script z e" "stop?timeout=1" 0
}

main