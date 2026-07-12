#!/usr/bin/env bash
# Wire a git credential helper for cross-owner GitHub access when a token is
# provided via the $GH_PAT environment variable. No-op when $GH_PAT is unset, so
# it's safe to run everywhere (local and cloud). See docs/cross-owner-clone.md.
#
# The egress proxy overrides auth for the session's own repos and *forwards* a
# caller-supplied token for repos it doesn't manage, so this helper only takes
# effect for cross-owner repos (e.g. skylartaylor/thinkpod) and is harmless for
# this repo. Only the helper *script text* lands in ~/.gitconfig; the token
# value is read from $GH_PAT at git runtime, so it never enters a cached image.
set -euo pipefail

[ -n "${GH_PAT:-}" ] || exit 0

git config --global credential.helper \
  '!f() { echo username=x-access-token; echo "password=$GH_PAT"; }; f'
