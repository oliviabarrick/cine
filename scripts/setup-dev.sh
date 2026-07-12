#!/usr/bin/env bash
# Installs pinned development dependencies (besides Go itself) via `go install`.
# Versions here should stay in sync with .github/workflows/ci.yml.
set -euo pipefail

if ! command -v go >/dev/null; then
  echo "Go is not installed. Install the version from go.mod: $(grep '^go ' go.mod | awk '{print $2}')" >&2
  echo "See https://go.dev/dl/" >&2
  exit 1
fi

echo "Installing helm..."
go install helm.sh/helm/v3/cmd/helm@v3.16.4

echo "Installing yq..."
go install github.com/mikefarah/yq/v4@v4.44.3

echo
echo "Done. Make sure $(go env GOPATH)/bin is on your PATH."
