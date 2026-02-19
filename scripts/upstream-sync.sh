#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<EOF2
Usage: ./scripts/upstream-sync.sh [--branch sync/upstream-<date>] [--dry-run]

Options:
  --branch <name>  Override sync branch name (default: sync/upstream-YYYYMMDD)
  --dry-run        Print planned actions without mutating git state
  -h, --help       Show this help
EOF2
}

DRY_RUN=false
BRANCH="sync/upstream-$(date -u +%Y%m%d)"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)
      DRY_RUN=true
      shift
      ;;
    --branch)
      BRANCH="${2:-}"
      if [[ -z "$BRANCH" ]]; then
        echo "Error: --branch requires a value" >&2
        exit 1
      fi
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Error: unknown argument '$1'" >&2
      usage
      exit 1
      ;;
  esac
done

if [[ ! "$BRANCH" =~ ^sync/upstream-[0-9]{8}$ && ! "$BRANCH" =~ ^sync/upstream-[A-Za-z0-9._/-]+$ ]]; then
  echo "Error: invalid branch name '$BRANCH'" >&2
  exit 1
fi

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  echo "Error: run this from inside a git repository" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
  echo "Error: working tree is not clean. Commit or stash changes first." >&2
  exit 1
fi

echo "[sync] repository: $(basename "$(git rev-parse --show-toplevel)")"
echo "[sync] branch: $BRANCH"
echo "[sync] mode: $([[ "$DRY_RUN" == "true" ]] && echo dry-run || echo execute)"

if [[ "$DRY_RUN" == "true" ]]; then
  cat <<EOF2
Planned actions:
1. git fetch --no-tags origin main
2. git fetch --no-tags upstream main
3. Verify upstream ancestry against origin/main
4. git switch -c $BRANCH origin/main
5. git merge --no-ff --no-edit upstream/main
6. Print push + PR commands
EOF2
  exit 0
fi

git fetch --no-tags origin main
git fetch --no-tags upstream main

if git merge-base --is-ancestor upstream/main origin/main; then
  echo "No sync needed: upstream/main has no new commits beyond origin/main"
  exit 0
fi

if git show-ref --verify --quiet "refs/heads/$BRANCH"; then
  echo "Error: local branch '$BRANCH' already exists." >&2
  echo "Delete it or rerun with --branch sync/upstream-<newdate>." >&2
  exit 1
fi

git switch -c "$BRANCH" origin/main

echo "[sync] attempting merge from upstream/main..."
if git merge --no-ff --no-edit upstream/main; then
  if git diff --quiet origin/main..HEAD; then
    echo "No content changes after merge; cleaning local sync branch."
    git switch main
    git branch -D "$BRANCH"
    exit 0
  fi

  cat <<EOF2
Merge completed successfully.

Next steps:
1. git push -u origin $BRANCH
2. gh pr create --base main --head $BRANCH \\
   --title "chore(sync): upstream gastown $(date -u +%Y-%m-%d)" \\
   --body "Automated upstream sync from steveyegge/gastown." \\
   --label upstream-sync --label needs-review
EOF2
else
  echo "Merge conflict detected. Manual recovery required." >&2
  cat <<EOF2 >&2

Recovery path:
1. Resolve conflicts in this branch and commit.
2. Push fixed branch:
   git push -u origin $BRANCH
3. Open PR to main with labels upstream-sync, needs-review.

Rollback path:
1. Abort merge:
   git merge --abort
2. Switch away and delete branch:
   git switch main
   git branch -D $BRANCH
3. Re-run script with a fresh branch name.
EOF2
  exit 1
fi
