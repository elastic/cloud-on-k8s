#!/bin/bash -eu

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script to test the destruction of child processes (especially zombies processes).
# Usage:
#    ./simulate-child-processes.sh                                   Run the script without creating child processes
#    ./simulate-child-processes.sh zombies                           Run the script and create zombies child processes
#    ./simulate-child-processes.sh zombies enableTrap                Run the script, create zombies processes and trap stop signals


withZombies=${1:-""}
enableTrap=${2:-""}

sub_processes() {
    while true; do
        echo "sub > sleep 3s... ($(date +%s))"
        sleep 3
    done
}
create_zombie() {
    if [[ "$withZombies" != "" ]]; then
        echo "Creating a zombie..."
        (sleep 1 & exec /bin/sleep 600) &
    fi
}

on_trap() {
    echo "Signal trapped"
    while true; do
        create_zombie
        echo "main/trap > sleep 4s... ($(date +%s))"
        sleep 4
    done
}

main() {
    if [[ "$enableTrap" != "" ]]; then
        trap on_trap EXIT SIGHUP SIGINT SIGQUIT SIGABRT SIGTERM
    fi

    sub_processes &

    while true; do
        create_zombie
        echo "main > sleep 3s... ($(date +%s))"
        sleep 3
    done
}

main
