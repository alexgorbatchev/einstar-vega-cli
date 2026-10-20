#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
just --justfile ./justfile setup
just --justfile ./justfile setup
if output=$(VEGA_IP="" just --justfile ./justfile run </dev/null 2>&1); then
    echo "expected the missing device address to fail" >&2
    exit 1
fi
[[ "$output" == *"--ip is required"* ]]
