#!/usr/bin/env bash

# Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
# or more contributor license agreements. Licensed under the Elastic License 2.0;
# you may not use this file except in compliance with the Elastic License 2.0.

# Script to generate API reference documentation from the source code.
# To test with a different version of crd-ref-docs while retaining the binary, invoke the script as follows:
# SCRATCH_DIR=/tmp/crd-ref-docs-tmp CLEANUP=false REFDOCS_VER=e36d311 ./build.sh

set -euo pipefail

SCRATCH_DIR="${SCRATCH_DIR:-$(mktemp -d -t crd-ref-docs-XXXXX)}"
CLEANUP="${CLEANUP:-true}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Load the version script to get the current version of the project.
# shellcheck disable=SC1091
source "${SCRIPT_DIR}"/../version.sh

cleanup() {
    if [[ $CLEANUP == "true" ]]; then
        echo "Removing $SCRATCH_DIR"
        rm -rf "$SCRATCH_DIR" || echo "Failed to remove $SCRATCH_DIR"
    fi
}

build_docs() {
    local REPO_ROOT="${SCRIPT_DIR}/../.."
    local DOCS_DIR="${SCRIPT_DIR}/../../docs"
    local REFDOCS_REPO="${REFDOCS_REPO:-github.com/elastic/crd-ref-docs}"
    local REFDOCS_VER="${REFDOCS_VER:-v0.3.0}"
    local BIN_DIR=${SCRATCH_DIR}/bin

    local version
    version="$(get_current_version)"
    # Remove dots from the version string for compatibility with the doc web site.
    local outFile="${version//./_}.md"

    local SCRATCH_TEMPLATES="${SCRATCH_DIR}/templates"
    local OUT_PATH="${DOCS_DIR}/reference/api-reference/${outFile}"

    (
        echo "Installing crd-ref-docs $REFDOCS_VER to $BIN_DIR"
        mkdir -p "$BIN_DIR"
        GOBIN=$BIN_DIR go install "${REFDOCS_REPO}@${REFDOCS_VER}"

        echo "Staging templates and injecting URL mapping"
        mkdir -p "${SCRATCH_TEMPLATES}"
        cp -R "${SCRIPT_DIR}/templates/." "${SCRATCH_TEMPLATES}/"
        MAPPING_PATH="${SCRIPT_DIR}/url-mapping.json" \
        TEMPLATES_DIR="${SCRATCH_TEMPLATES}" \
        python3 - <<'PYEOF'
import json, os, pathlib
m = json.load(open(os.environ["MAPPING_PATH"]))
q = lambda s: s.replace("\\", "\\\\").replace('"', '\\"')
pipe = "".join(' | replace "%s" "[%s](%s)"' % (q(u), q(e["text"]), q(e["link"])) for u, e in m["mappings"].items())
for fname, expr, tail in [("type_members.tpl", "$field.Doc", ' | replace "\\n" "<br>"'), ("type.tpl", "$type.Doc", ""), ("gv_details.tpl", "$gv.Doc", "")]:
    p = pathlib.Path(os.environ["TEMPLATES_DIR"]) / fname
    src, old = p.read_text(), "{{ "+expr+tail+" }}"
    if old not in src: raise SystemExit(f"pattern not found in {fname}: {old!r}")
    p.write_text(src.replace(old, "{{ "+expr+pipe+tail+" }}"))
PYEOF

        echo "Generating API reference documentation for version: ${version}, output file: ${outFile}"
        "${BIN_DIR}"/crd-ref-docs --source-path="${REPO_ROOT}"/pkg/apis \
            --config="${SCRIPT_DIR}"/config.yaml \
            --renderer=markdown \
            --template-value=eckVersion="${version}" \
            --templates-dir="${SCRATCH_TEMPLATES}" \
            --output-path="${OUT_PATH}"
    )

    if grep -nE 'https://www\.elastic\.co/docs/' "${OUT_PATH}" >/dev/null; then
        echo "WARNING: unmapped elastic.co/docs URL(s) in ${OUT_PATH} (add to url-mapping.json):" >&2
        grep -nE 'https://www\.elastic\.co/docs/' "${OUT_PATH}" >&2
    fi
}

trap cleanup EXIT
build_docs
