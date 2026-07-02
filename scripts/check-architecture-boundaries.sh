#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

module_path="$(go list -m)"
root_pkg="${module_path}/pkg/agentcompose"

packages=(
  "./pkg/agentcompose/..."
  "./internal/..."
  "./cmd/..."
)

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

imports_file="$tmpdir/imports.tsv"
go list -f '{{.ImportPath}}{{range .Imports}}{{printf "\t%s" .}}{{end}}' "${packages[@]}" >"$imports_file"

failures=0

report_failure() {
  local pkg="$1"
  local import_path="$2"
  local reason="$3"
  printf 'architecture boundary violation: %s imports %s: %s\n' "$pkg" "$import_path" "$reason" >&2
  failures=$((failures + 1))
}

is_agentcompose_subpackage() {
  local pkg="$1"
  [[ "$pkg" == "${root_pkg}/"* ]]
}

while IFS=$'\t' read -r pkg imports; do
  IFS=$'\t' read -r -a fields <<<"${pkg}${imports:+$'\t'}${imports:-}"
  pkg="${fields[0]}"
  for ((i = 1; i < ${#fields[@]}; i++)); do
    import_path="${fields[$i]}"

    if is_agentcompose_subpackage "$pkg" && [[ "$import_path" == "$root_pkg" ]]; then
      report_failure "$pkg" "$import_path" "subpackage must not depend on the root compatibility package"
    fi

    if [[ "$pkg" == "${root_pkg}/store/"* ]]; then
      case "$import_path" in
        "${root_pkg}/transport"*|"${root_pkg}/app")
          report_failure "$pkg" "$import_path" "store packages must not depend on app or transport packages"
          ;;
      esac
    fi

    if is_agentcompose_subpackage "$pkg" && [[ "$pkg" != "${root_pkg}/transport"* && "$pkg" != "${root_pkg}/app" ]]; then
      case "$import_path" in
        "${root_pkg}/transport"*|"${root_pkg}/app")
          report_failure "$pkg" "$import_path" "core packages must not depend on app or transport packages"
          ;;
      esac
    fi

    if [[ "$pkg" == "${root_pkg}/transport"* ]]; then
      case "$import_path" in
        "${root_pkg}/store/"*)
          report_failure "$pkg" "$import_path" "transport packages must not depend directly on concrete store packages"
          ;;
      esac
    fi
  done
done <"$imports_file"

if ((failures > 0)); then
  printf 'architecture boundary check failed with %d violation(s)\n' "$failures" >&2
  exit 1
fi

printf 'architecture boundary check passed\n'
