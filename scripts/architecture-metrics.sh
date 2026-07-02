#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

scope_patterns=(
  "./cmd/..."
  "./internal/..."
  "./pkg/agentcompose/..."
)

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

packages_file="$tmpdir/packages.txt"
go list -f '{{.ImportPath}} {{.Dir}} {{len .GoFiles}} {{len .TestGoFiles}}' "${scope_patterns[@]}" >"$packages_file"

now="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"
commit="$(git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)"
branch="$(git branch --show-current 2>/dev/null || echo unknown)"

cat <<EOF
# Architecture Metrics Baseline

Generated: ${now}

Branch: \`${branch}\`

Commit: \`${commit}\`

Scope:

- \`./cmd/...\`
- \`./internal/...\`
- \`./pkg/agentcompose/...\`

## Package Size

| Go LOC | Go files | Test files | Package |
| ---: | ---: | ---: | --- |
EOF

while read -r import_path dir go_files test_files; do
  loc="$(find "$dir" -maxdepth 1 -name '*.go' -print0 | xargs -0 wc -l 2>/dev/null | awk 'END {print $1 + 0}')"
  printf '| %s | %s | %s | `%s` |\n' "$loc" "$go_files" "$test_files" "$import_path"
done <"$packages_file" | sort -t '|' -k2,2nr | head -50

cat <<'EOF'

## Largest Production Files

| Lines | File |
| ---: | --- |
EOF

find cmd internal pkg/agentcompose -name '*.go' \
  -not -name '*_test.go' \
  -print0 |
  xargs -0 wc -l |
  sort -nr |
  awk 'NR <= 30 && $2 != "total" { printf("| %s | `%s` |\n", $1, $2) }'

cat <<'EOF'

## Largest Test Files

| Lines | File |
| ---: | --- |
EOF

find cmd internal pkg/agentcompose -name '*_test.go' \
  -print0 |
  xargs -0 wc -l |
  sort -nr |
  awk 'NR <= 30 && $2 != "total" { printf("| %s | `%s` |\n", $1, $2) }'

cat <<'EOF'

## Root Package Inventory

| Lines | File |
| ---: | --- |
EOF

find pkg/agentcompose -maxdepth 1 -name '*.go' \
  -print0 |
  xargs -0 wc -l |
  sort -nr |
  awk '$2 != "total" { printf("| %s | `%s` |\n", $1, $2) }'

cat <<'EOF'

## Warning Thresholds

- Production file above 600 lines: review for responsibility split.
- Production file above 1000 lines: split unless there is a documented reason not to.
- Package above 3000 lines: review exported API, cohesion, and dependency direction.
- Root `pkg/agentcompose` should trend downward during hardening.
EOF
