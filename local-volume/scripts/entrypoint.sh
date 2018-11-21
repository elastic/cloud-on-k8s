#!/usr/bin/env bash

case "$1" in
    driver) ./driver.sh;;
    provisioner) ./provisioner;;
    *) echo "Usage: entrypoint.sh <driver|provisioner>";;
esac