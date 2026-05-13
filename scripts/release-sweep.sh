#!/usr/bin/env bash
# release-sweep.sh — bump semver tags across sibling repos, each from its own current version
# Usage:
#   release-sweep.sh [patch|minor|major] [--dry-run] [--yes] [--allow-dirty] [--root DIR]
#   release-sweep.sh [patch|minor|major] [--dry-run] [--yes] [--allow-dirty] [--root DIR] [--repos a,b,c]
#   release-sweep.sh [patch|minor|major] [--dry-run] [--yes] [--allow-dirty] [--root DIR] repo1 repo2 ...
# Major releases promote pre-v1 repos to v1.0.0 even if there are no commits since the last tag.
set -euo pipefail

BOLD='\033[1m'
DIM='\033[2m'
GREEN='\033[32m'
YELLOW='\033[33m'
RED='\033[31m'
RESET='\033[0m'

usage() {
	cat <<'EOF'
Usage:
  release-sweep.sh [patch|minor|major] [--dry-run] [--yes] [--allow-dirty] [--root DIR] [--repos a,b,c] [repo ...]

Behavior:
  Scans top-level git repos under DIR (default: parent of the current repo),
  computes the next semver tag in each repo independently, shows a summary,
  and tags/pushes repos that have commits since their last semver tag.
  For a major bump, repos below v1.0.0 are also eligible for promotion to
  v1.0.0 even with no new commits.

Options:
  patch|minor|major  Semver bump type to apply in each repo (default: patch)
  --dry-run          Show what would happen without tagging/pushing
  --yes              Skip confirmation prompt
  --allow-dirty      Allow tagging repos with local changes
  --root DIR         Suite root containing child repos
  --repos LIST       Comma-separated repo names to target (blank = all)
  -h, --help         Show this help
EOF
}

die() {
	echo -e "${RED}Error:${RESET} $*" >&2
	exit 1
}

bump_tag() {
	local current="$1"
	local bump="$2"
	local ver major minor patch rest

	ver="${current#v}"
	major="${ver%%.*}"
	rest="${ver#*.}"
	minor="${rest%%.*}"
	patch="${rest##*.}"

	case "$bump" in
		major)
			major=$((major + 1))
			minor=0
			patch=0
			;;
		minor)
			minor=$((minor + 1))
			patch=0
			;;
		patch)
			patch=$((patch + 1))
			;;
		*)
			die "Unsupported bump type: $bump"
			;;
	esac

	printf 'v%s.%s.%s\n' "$major" "$minor" "$patch"
}

is_pre_v1() {
	local current="$1"
	local ver major

	ver="${current#v}"
	major="${ver%%.*}"
	[[ "$major" =~ ^[0-9]+$ ]] || return 1
	(( major < 1 ))
}

repo_remote() {
	local branch="$1"
	local upstream

	upstream="$(git config --get "branch.$branch.remote" || true)"
	if [[ -n "$upstream" ]]; then
		printf '%s\n' "$upstream"
		return
	fi
	if git remote get-url origin >/dev/null 2>&1; then
		printf 'origin\n'
		return
	fi
	git remote | head -1 || true
}

append_repo_list() {
	local raw="$1"
	local normalized token

	normalized="$(printf '%s' "$raw" | tr ',' ' ')"
	[[ -n "$(printf '%s' "$normalized" | xargs)" ]] || return 0

	for token in $normalized; do
		case "$token" in
			all|'*')
				return 0
				;;
		esac
		REPOS+=("$token")
	done
}

BUMP="patch"
DRY_RUN=false
ASSUME_YES=false
ALLOW_DIRTY=false
ROOT=""
REPOS=()

while [[ $# -gt 0 ]]; do
	case "$1" in
		patch|minor|major)
			BUMP="$1"
			;;
		--dry-run)
			DRY_RUN=true
			;;
		--yes|-y)
			ASSUME_YES=true
			;;
		--allow-dirty)
			ALLOW_DIRTY=true
			;;
		--root)
			shift
			[[ $# -gt 0 ]] || die "--root requires a value"
			ROOT="$1"
			;;
		--repos)
			shift
			[[ $# -gt 0 ]] || die "--repos requires a value"
			append_repo_list "$1"
			;;
		--repos=*)
			append_repo_list "${1#--repos=}"
			;;
		-h|--help)
			usage
			exit 0
			;;
		--)
			shift
			while [[ $# -gt 0 ]]; do
				REPOS+=("$1")
				shift
			done
			break
			;;
		-*)
			die "Unknown argument: $1"
			;;
		*)
			REPOS+=("$1")
			;;
	esac
	shift
done

git rev-parse --git-dir >/dev/null 2>&1 || die "Run this from inside a git repo, or pass --root"

if [[ -z "$ROOT" ]]; then
	ROOT="$(dirname "$(git rev-parse --show-toplevel)")"
fi
ROOT="$(cd "$ROOT" && pwd)"

[[ -d "$ROOT" ]] || die "Root does not exist: $ROOT"

SUITE_REPOS=()
while IFS= read -r repo_dir; do
	[[ -n "$repo_dir" ]] || continue
	SUITE_REPOS+=("$repo_dir")
done < <(find "$ROOT" -mindepth 1 -maxdepth 1 -type d -exec test -d '{}/.git' ';' -print | sort)

[[ ${#SUITE_REPOS[@]} -gt 0 ]] || die "No top-level git repos found under $ROOT"

declare -A WANTED=()
if [[ ${#REPOS[@]} -gt 0 ]]; then
	for repo in "${REPOS[@]}"; do
		WANTED["$repo"]=1
	done
fi

TARGET_REPOS=()
for repo_dir in "${SUITE_REPOS[@]}"; do
	repo_name="$(basename "$repo_dir")"
	if [[ ${#WANTED[@]} -gt 0 && -z "${WANTED[$repo_name]:-}" ]]; then
		continue
	fi
	TARGET_REPOS+=("$repo_dir")
done

if [[ ${#TARGET_REPOS[@]} -eq 0 ]]; then
	available="$(printf '%s\n' "${SUITE_REPOS[@]##*/}" | paste -sd ', ' -)"
	die "No matching repos selected. Available repos: $available"
fi

PLANNED=()
SKIPPED=()

echo -e "${BOLD}Release Sweep${RESET} — $(date '+%Y-%m-%d %H:%M')"
echo -e "  ${DIM}Root:${RESET} $ROOT"
echo -e "  ${DIM}Mode:${RESET} $BUMP"
echo ""

for repo_dir in "${TARGET_REPOS[@]}"; do
	repo_name="$(basename "$repo_dir")"
	cd "$repo_dir"

	branch="$(git branch --show-current 2>/dev/null || true)"
	if [[ -z "$branch" ]]; then
		SKIPPED+=("$repo_name|detached HEAD")
		continue
	fi

	if ! $ALLOW_DIRTY && [[ -n "$(git status --porcelain)" ]]; then
		SKIPPED+=("$repo_name|dirty working tree")
		continue
	fi

	current="$(git tag --merged HEAD --sort=-v:refname | grep -E '^v?[0-9]+\.[0-9]+\.[0-9]+$' | head -1 || true)"
	if [[ -z "$current" ]]; then
		current="v0.0.0"
		log="$(git log -n 20 --oneline --no-decorate)"
	else
		[[ "$current" == v* ]] || current="v$current"
		log="$(git log "${current}..HEAD" --oneline --no-decorate)"
	fi

	new_tag="$(bump_tag "$current" "$BUMP")"
	allow_v1_promotion=false
	if [[ "$BUMP" == "major" ]] && is_pre_v1 "$current" && [[ "$new_tag" == "v1.0.0" ]]; then
		allow_v1_promotion=true
	fi

	if [[ -z "$log" ]]; then
		if ! $allow_v1_promotion; then
			SKIPPED+=("$repo_name|no commits since $current")
			continue
		fi
	fi

	if git rev-parse -q --verify "refs/tags/$new_tag" >/dev/null 2>&1; then
		SKIPPED+=("$repo_name|tag already exists: $new_tag")
		continue
	fi

	commit_count="$(printf '%s\n' "$log" | sed '/^$/d' | wc -l | tr -d ' ')"
	remote="$(repo_remote "$branch")"
	head_sha="$(git rev-parse --short HEAD)"
	first_commit="$(printf '%s\n' "$log" | head -1)"
	if [[ -z "$first_commit" ]]; then
		first_commit="(no new commits; v1 promotion)"
	fi
	PLANNED+=("$repo_dir|$repo_name|$branch|$current|$new_tag|$commit_count|$remote|$head_sha|$first_commit")

	echo -e "${BOLD}${repo_name}${RESET}"
	echo -e "  ${DIM}Branch:${RESET} $branch"
	echo -e "  ${DIM}HEAD:${RESET} $head_sha"
	echo -e "  ${DIM}Current:${RESET} $current"
	echo -e "  ${GREEN}Next:${RESET} $new_tag"
	echo -e "  ${DIM}Commits:${RESET} $commit_count"
	echo -e "  ${DIM}Latest:${RESET} $first_commit"
	echo ""
done

if [[ ${#SKIPPED[@]} -gt 0 ]]; then
	echo -e "${BOLD}Skipped${RESET}"
	for item in "${SKIPPED[@]}"; do
		repo_name="${item%%|*}"
		reason="${item#*|}"
		echo -e "  ${YELLOW}-${RESET} $repo_name (${reason})"
	done
	echo ""
fi

if [[ ${#PLANNED[@]} -eq 0 ]]; then
	echo -e "${YELLOW}No repos need tagging.${RESET}"
	exit 0
fi

if $DRY_RUN; then
	echo -e "${BOLD}Dry Run Commands${RESET}"
	for item in "${PLANNED[@]}"; do
		IFS='|' read -r repo_dir repo_name branch current new_tag commit_count remote head_sha first_commit <<< "$item"
		if [[ -n "$remote" ]]; then
			echo -e "  ${YELLOW}${repo_name}:${RESET} git -C $repo_dir tag $new_tag && git -C $repo_dir push $remote $new_tag"
		else
			echo -e "  ${YELLOW}${repo_name}:${RESET} git -C $repo_dir tag $new_tag"
		fi
	done
	exit 0
fi

if ! $ASSUME_YES; then
	printf "Tag and push %d repo(s)? [y/N] " "${#PLANNED[@]}"
	read -r ans
	[[ "$ans" =~ ^[Yy]([Ee][Ss])?$ ]] || {
		echo "Aborted."
		exit 0
	}
fi

for item in "${PLANNED[@]}"; do
	IFS='|' read -r repo_dir repo_name branch current new_tag commit_count remote head_sha first_commit <<< "$item"
	echo -e "${BOLD}Tagging${RESET} $repo_name -> $new_tag"
	git -C "$repo_dir" tag "$new_tag"
	if [[ -n "$remote" ]]; then
		git -C "$repo_dir" push "$remote" "$new_tag"
		echo -e "  ${GREEN}✓${RESET} pushed to $remote"
	else
		echo -e "  ${YELLOW}!${RESET} no remote configured; tag created locally"
	fi
done

echo ""
echo -e "${GREEN}Done:${RESET} tagged ${#PLANNED[@]} repo(s)"
