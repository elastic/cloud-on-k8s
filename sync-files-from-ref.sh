#!/usr/bin/env bash
#
# Copy every go.mod, go.sum and Dockerfile from another git ref (default: main)
# into the current working tree. The files are left staged, nothing is committed.
#
# Usage: ./sync-deps-from-ref.sh [ref]

set -euo pipefail

ref="${1:-main}"
pattern='(^|/)(go\.mod|go\.sum)$|(^|/)[^/]*Dockerfile[^/]*$'

cd "$(git rev-parse --show-toplevel)"

git rev-parse --verify --quiet "${ref}^{commit}" >/dev/null || {
    echo "error: unknown git ref '${ref}'" >&2
    exit 1
}

# Refuse to overwrite uncommitted changes in the files we are about to replace.
dirty="$(git status --porcelain --untracked-files=no | cut -c4- | grep -E "$pattern" || true)"
if [[ -n "$dirty" ]]; then
    echo "error: uncommitted changes in files that would be overwritten:" >&2
    echo "$dirty" >&2
    exit 1
fi

files=()
while IFS= read -r f; do
    files+=("$f")
done < <(git ls-tree -r --name-only "$ref" | grep -E "$pattern" || true)

if [[ ${#files[@]} -eq 0 ]]; then
    echo "no go.mod, go.sum or Dockerfile found in '${ref}'"
    exit 0
fi

echo "copying ${#files[@]} files from '${ref}':"
printf '  %s\n' "${files[@]}"
echo

git checkout "$ref" -- "${files[@]}"

# Files that exist here but not in the ref are left untouched; report them.
extra="$(comm -23 <(git ls-files | grep -E "$pattern" | sort) <(printf '%s\n' "${files[@]}" | sort))"
if [[ -n "$extra" ]]; then
    echo "not in '${ref}', left untouched:"
    echo "$extra"
    echo
fi

echo "copied ${#files[@]} files from '${ref}' ($(git rev-parse --short "$ref")); changed:"
git diff --cached --stat -- "${files[@]}"
