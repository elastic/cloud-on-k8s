#!/bin/bash -eu

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License;
# you may not use this file except in compliance with the Elastic License.

# Script to test the destruction of child processes (especially zombies processes).
# Usage:
#    ./simulate-child-processes.sh                                   Run the script without creating child processes
#    ./simulate-child-processes.sh zombies                           Run the script and create zombies child processes
#    ./simulate-child-processes.sh zombies enableTrap                Run the script, create zombies processes and trap stop signals


forever=${1:-""}
enableTrap=${2:-""}
foreverAfterTrap=${3:-""}

sub_processes() {
    while true; do
        echo "sub > sleep 3s... ($(date +%s))"
        sleep 3
    done
}

create_zombie() {
    echo "Creating a zombie..."
    (sleep 1 & exec /bin/sleep 600) &
}

on_trap() {
    echo "Signal trapped"
    if [[ "$foreverAfterTrap" != "" ]]; then
       # Never stops
       while true; do
            create_zombie
            echo "trap/forever > sleep 2s... ($(date +%s))"
            sleep 2
       done
    else
        #create_zombie
        echo "trap/once > sleep 2s... ($(date +%s))"
        sleep 2
    fi
}

main() {
    if [[ "$enableTrap" != "" ]]; then
        trap on_trap EXIT SIGHUP SIGINT SIGQUIT SIGABRT SIGTERM
    fi

    #sub_processes &

    if [[ "$forever" != "" ]]; then
        while true; do
            create_zombie
            echo "forever > sleep 3s... ($(date +%s))"
            sleep 3
        done
    else
        #create_zombie
        echo "once > sleep 3s... ($(date +%s))"
        sleep 3
    fi

    echo "End of execution"
}

main
