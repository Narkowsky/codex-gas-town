# Using codex-gas-town as Central Runtime

`codex-gas-town` is the central orchestration/runtime fork.
Application code should remain in separate project repositories.

## Hard rules

1. This repo is a **control-plane/runtime** repository, not a feature mono-repo.
2. Project application code is developed in project-specific repos.
3. Upstream updates from `steveyegge/gastown` flow through sync PRs only.
4. `main` stays protected and merge-only via PR.

## Downstream project onboarding checklist

For each new app repo:

1. Clone/update this repo:
   - `git clone git@github.com:Narkowsky/codex-gas-town.git`
2. Install/runtime bootstrap per upstream `gastown` docs.
3. Register the target app repository path as a workspace/project in your local runtime config.
4. Run one smoke command against the target app repo to verify runtime connectivity and toolchain.
5. Document the workspace mapping in that project’s local ops notes.

## Version pin policy

When downstream repos reference this control plane:

1. Pin to a specific release tag or commit SHA from `codex-gas-town`.
2. Do not pin to floating `main` or `latest` for unattended automation.
3. Upgrade downstream pins only after sync PR review and smoke verification.

## Recommended operating model

1. Weekly upstream sync PR review in this repo.
2. After merge, run control-plane smoke checks.
3. Roll out pinned update to downstream repos deliberately.
4. Track incidents and sync regressions in this repo’s issues.
