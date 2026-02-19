# Upstream Sync Operations

This repository is a fork of `steveyegge/gastown` and syncs upstream changes into `main` via pull requests.

## Automated weekly sync

- Workflow: `.github/workflows/upstream-sync.yml`
- Trigger schedule: every Monday at 08:00 UTC
- Manual trigger: GitHub Actions -> **Upstream Sync** -> **Run workflow**

Behavior:
1. Fetch `origin/main` and `upstream/main`.
2. Exit cleanly if no diff.
3. Create/update `sync/upstream-YYYYMMDD` branch.
4. Merge `upstream/main` into sync branch.
5. Push sync branch and open/update PR to `main` with labels:
   - `upstream-sync`
   - `needs-review`

## Manual sync

Use when you want immediate update or to resolve failed automation.

```bash
./scripts/upstream-sync.sh
```

Optional flags:

```bash
./scripts/upstream-sync.sh --dry-run
./scripts/upstream-sync.sh --branch sync/upstream-20260219
```

## Conflict handling

When automated sync hits conflicts:
1. Workflow opens an issue titled `Upstream sync conflict: YYYY-MM-DD` with label `upstream-sync-conflict`.
2. Create a manual branch from `origin/main`.
3. Merge `upstream/main`, resolve conflicts, commit, push.
4. Open PR to `main` with labels `upstream-sync` and `needs-review`.

## Rollback path

If you need to discard a failed sync branch:

1. Close related sync PR (if opened).
2. Delete remote sync branch:
   - `git push origin --delete sync/upstream-<date>`
3. Delete local sync branch:
   - `git branch -D sync/upstream-<date>`
4. Re-run automated workflow or manual script.

## Safety rules

1. Never merge upstream directly into `main` without PR.
2. Keep `main` protected; resolve all conflicts in sync branches.
3. Prefer squash-merge for sync PRs.
