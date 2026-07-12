# Cloning & pushing a cross-owner repo (e.g. `skylartaylor/thinkpod`)

Working on Cine often needs the deploy repo `skylartaylor/thinkpod`, which lives
under a **different owner**. A plain `git clone` of it fails from a `cine`
session — not because of egress (`github.com` is reachable), but because the
session has no credential for `skylartaylor`. Given a `GH_PAT` env var holding a
token that can access the target repo, here's how to clone and push. (Providing
that token is out of scope here.)

## Clone

If a global credential helper is already configured (i.e. `$GH_PAT` was present
at session start), just:

```sh
git clone https://github.com/skylartaylor/thinkpod
```

Otherwise, apply the helper for a one-off clone — **never** put the token in the
remote URL (`https://x-access-token:$GH_PAT@github.com/…` gets persisted verbatim
into `.git/config`):

```sh
HELPER='!f() { echo username=x-access-token; echo "password=$GH_PAT"; }; f'
git -c credential.helper="$HELPER" clone https://github.com/skylartaylor/thinkpod
git -C thinkpod config credential.helper "$HELPER"   # so fetch/push reuse it
```

You get a full working tree — history and large files included.

## Push

Normal git; the credential helper above covers auth (needs a token with write):

```sh
cd thinkpod
git checkout -b my-branch
git add -A && git commit -m "…"
git push -u origin my-branch
```

## Why this works / gotchas

- The egress proxy **forwards `$GH_PAT` to `github.com` for repos it doesn't
  manage** (like `skylartaylor/thinkpod`) and injects its own scoped token for
  this session's own repos — so the helper only takes effect where it's needed
  and is harmless for `cine`. Keep the proxy CA in place; don't disable TLS.
- Use the capital-G **`mcp__Github__*`** connector for GitHub API reads — the
  lowercase `mcp__github__*` is scoped to this session's repo and returns
  "access denied" for other owners (not a real reachability block).
- `add_repo <other-owner>/<repo>` does **not** work — cross-tier adds are
  rejected; a session is pinned to one owner tier.
- `git`-over-HTTPS on `github.com` and `raw.githubusercontent.com` are reachable;
  direct `api.github.com` and `codeload.github.com` are egress-blocked (403).
