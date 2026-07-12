# Getting a local clone of a cross-owner repo (e.g. `skylartaylor/thinkpod`)

When working on Cine from a Claude Code / remote-execution session you often
also need the deploy repo `skylartaylor/thinkpod` (it owns `fleet.yaml`, the
factory templates, and this repo's generated CI — see `.github/CLAUDE.md`). A
naive `git clone` of it from a `cine` session fails. This note explains **why**,
and gives **two working ways** to get the files locally — a real git clone, and
a PAT-free read-only materialization.

## TL;DR

- The failure is **credential scoping, not the egress proxy**. `github.com` is
  reachable from local git; this session just has no token for `skylartaylor`.
- The egress proxy **forwards your own `Authorization` header** for repos it
  doesn't manage (verified below), so a **real `git clone` works** once git
  presents a token that can read `skylartaylor/thinkpod` — full history, large
  files, and ordinary `git push`. See **Method 1**.
- With no token, you can still get the full tree locally read-only via the
  capital-G GitHub MCP connector plus signed `raw.githubusercontent.com` URLs.
  See **Method 2**.
- Don't reach for the lowercase `mcp__github__*` connector, unset the proxy, or
  disable TLS — none of those is the blocker.

## What is and isn't reachable (measured, not assumed)

All probes were run from inside a `cine` session through the local egress proxy
(`$HTTPS_PROXY`). The proxy sets `GITHUB_TOKEN=proxy-injected`: there is no real
token in the env or git config; the proxy injects a GitHub `Authorization`
header on the fly, **scoped to the repos this session is authorized for**
(`oliviabarrick/cine`).

| Request through the proxy | Result | What it proves |
| --- | --- | --- |
| `git ls-remote github.com/oliviabarrick/cine` with a **bogus** inline token | **succeeds** | For an authorized repo the proxy **discards your header and injects its own** valid token. |
| `git ls-remote github.com/skylartaylor/thinkpod` with a **bogus** inline token | GitHub 401 `Invalid username or token` | For a repo it doesn't manage the proxy **forwards your credential unchanged** — so a *valid* token would authenticate. |
| `git ls-remote github.com/skylartaylor/thinkpod` with **no** credential | `could not read Username … terminal prompts disabled` | Plain missing-credential, **not** a blocked host. |
| `curl https://raw.githubusercontent.com/skylartaylor/thinkpod/<sha>/<path>?token=…` (signed) | **HTTP 200** | `raw.githubusercontent.com` is reachable; MCP-issued signed blob URLs work. |
| `curl https://api.github.com/repos/skylartaylor/thinkpod` (direct, unauth) | **HTTP 403** | Direct `api.github.com` is egress-blocked — use the MCP connector instead. |
| `curl https://codeload.github.com/skylartaylor/thinkpod/tar.gz/…` | **HTTP 403** | The tarball endpoint is egress-blocked too. |

Takeaway: **`github.com` git-over-HTTPS is open and honours a caller-supplied
token for cross-owner repos.** That's the seam Method 1 uses.

## The MCP gotcha (why an earlier attempt "proved" it was blocked)

Per `.github/CLAUDE.md`: use the **capital-G `mcp__Github__*`** connector, not
lowercase `mcp__github__*`. The lowercase one is scoped to this session's
attached repo and returns `Access denied: repository "skylartaylor/thinkpod" is
not configured for this session`; the capital-G one uses your GitHub OAuth and
reaches `skylartaylor/thinkpod` fine. A wrong-connector "access denied" is not
evidence that the repo is unreachable.

## Method 1 — real `git clone` with a scoped token (preferred)

Because the proxy forwards your credential for `skylartaylor/thinkpod` and
`github.com` is reachable, a normal clone works once git has a usable token.

1. Create a token that can actually reach the target repo, and keep it
   short-lived:
   - **If the repo is owned by a different _user_ account** (as
     `skylartaylor/thinkpod` is, relative to `oliviabarrick`), use a **classic
     PAT with the `repo` scope**. A *fine-grained* PAT is owned by a single
     account and only reaches that account's resources, so a fine-grained token
     you create under your own account **cannot** read another user's repo even
     when you're a collaborator — the clone fails with `403 remote: Write access
     to repository not granted` (GitHub's catch-all for "this token has no
     access to this repo"), which is authentication *succeeding* and
     authorization then failing, not a proxy problem.
   - **If the repo is owned by an org** you belong to, a fine-grained PAT with
     that **org as the resource owner** and **Contents: read** (add write only
     to push) works too.
2. Clone without ever writing the token to disk or into the remote URL, by
   feeding it through a credential helper that reads an env var at runtime:

   ```sh
   export GH_PAT=<paste-fine-grained-PAT>          # not stored anywhere
   HELPER='!f() { echo username=x-access-token; echo "password=$GH_PAT"; }; f'

   git -c credential.helper="$HELPER" \
       clone https://github.com/skylartaylor/thinkpod

   # so later fetch/push in the clone reuse the same env-var helper:
   git -C thinkpod config credential.helper "$HELPER"
   ```

   Avoid the `https://x-access-token:$GH_PAT@github.com/…` URL form — git
   persists it verbatim into `.git/config`, leaking the token into the repo.
3. You now have a full working tree (history, large files) and can
   `git commit` / `git push` normally, subject to the PAT's scope.

Verified: a classic-PAT clone of `skylartaylor/thinkpod` from a `cine` session
completed normally through the proxy (4,183 objects, full working tree), and the
credential-helper form left **no token in `.git/config`**.

Notes:
- The proxy still re-terminates TLS, so keep its CA in place
  (`/root/.ccr/ca-bundle.crt`); don't disable verification.
- The container is ephemeral — the clone and the `GH_PAT` env var vanish when
  the session ends. Nothing durable holds the secret. Revoke the PAT once
  you're done regardless.

## Method 2 — PAT-free read-only materialization (via MCP)

If you can't mint a PAT and only need to read (or make small edits through the
MCP), reconstruct the tree locally without a git remote:

1. `mcp__Github__get_file_contents` (capital-G) on the repo root, or
   `get_repository_tree`, to enumerate paths. Each file entry includes a
   `download_url` on `raw.githubusercontent.com` carrying a **short-lived
   `?token=` signature**.
2. Fetch each `download_url` through the proxy (`curl`/WebFetch) into a local
   directory mirroring the paths. Those signed URLs return 200 (verified) and
   need no header auth.
3. Because the signed tokens expire in minutes, fetch promptly and re-list
   rather than reusing stale URLs; for a big tree, list-and-fetch in one pass.

This gives full local **file** context (fixing the "no checkout context" pain)
but **no `.git` history**, and writes still go back through the MCP
(`push_files` / `create_or_update_file`), which is slow and size-limited for
large blobs. For anything heavy, prefer Method 1.

## Providing the token to Claude Code on the web sessions

Per the Claude Code on the web docs, **there is no dedicated secrets store yet**:
environment variables and setup scripts are stored in the environment config and
are **visible to anyone who can edit that environment**. A token you add is
protected at "anyone who can edit this environment" level, not encrypted-secret
level — so the token's **scope is your real security control**. Anything running
in the session can read the variable, and GitHub egress is open, so an
injected/compromised task could exfiltrate it via `git push`. Mint the smallest
token that does the job.

1. **Mint a least-privilege token.** For a cross-*user* target like
   `skylartaylor/thinkpod`, a token created under your own account can only be a
   broad classic `repo` PAT (fine-grained is impossible — confirmed: the
   resource-owner picker only lists your own account, and `skylartaylor` is a
   separate user, not an org you belong to). Prefer instead a **fine-grained PAT
   created while logged in as `skylartaylor`**, scoped to **only `thinkpod`**,
   **Contents: read** (add write only to push). Same result for git, far smaller
   blast radius if the variable leaks. Short expiry, rotate.
2. **Add it as an environment variable.** Environment selector → Add/Edit
   environment → Environment variables (`.env` format, one `KEY=value` per line,
   **no quotes**):

   ```
   GH_PAT=<classic ghp_… or the skylartaylor-owned github_pat_…>
   ```
3. **Turn it into git auth without baking the secret into the cached image** —
   put a credential helper in either the environment **setup script** (runs as
   root before Claude; result is cached) or a repo **SessionStart hook** (also
   covers teleported-local sessions). Guard on the variable so it's a no-op when
   unset:

   ```bash
   # setup script, or a SessionStart hook script in the repo
   [ -n "$GH_PAT" ] && git config --global credential.helper \
     '!f() { echo username=x-access-token; echo "password=$GH_PAT"; }; f'
   ```

   This writes only the helper *script text* into `~/.gitconfig`; the token
   value is read from `$GH_PAT` at git runtime, so it never enters the cached
   snapshot — only the freshly-injected env var each session.
4. **Cross-owner clones then just work in-session**, no token on any command
   line: `git clone https://github.com/skylartaylor/thinkpod`. The proxy
   forwards `$GH_PAT` for repos it doesn't manage; for your own repos it keeps
   injecting its own scoped token, so the global helper is harmless there.

Don't commit the token or write it literally into the setup script — keep it in
the env var only. And revoke any token you've pasted into a chat/transcript.

## What does not work (don't retry these)

- `add_repo skylartaylor/thinkpod` → `cross-tier adds are not supported in v1`.
  A session is pinned to one owner tier; you can only `add_repo` **same-owner**
  repos.
- Direct `api.github.com` / `codeload.github.com` → 403 (egress-blocked). Use
  the capital-G MCP connector and `raw.githubusercontent.com` instead.
- Supplying a bogus/again-scoped token for an *authorized* repo — the proxy
  overrides it anyway; it changes nothing.
- Unsetting `HTTPS_PROXY` or disabling TLS verification — the proxy isn't the
  blocker, and this only breaks the trusted path.
