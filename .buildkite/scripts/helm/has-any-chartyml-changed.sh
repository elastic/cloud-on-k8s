#!/usr/bin/env bash

# get the names of the files changed between HEAD and the previous commit.
# bash version 3+.
# same command as filesChanged=( $(git diff --name-only HEAD~1...HEAD) ) 
# avoids SC2207
filesChanged=()
while IFS='' read -r line; do filesChanged+=("$line"); done < <(git diff --name-only HEAD~1...HEAD)

re='^deploy\/eck-[a-z]+[-]*[a-z]*\/Chart\.yaml$'

for i in "${filesChanged[@]}"; do
    if [[ ${i} =~ ${re} ]]; then
        exit 0
    fi
done

# No changed file contained 'Chart.yaml', failing.
exit 1
