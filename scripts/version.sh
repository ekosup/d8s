#!/usr/bin/env bash
# Single entry point for reading and changing the app version (.version).
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
file="$root/.version"

usage() {
	echo "usage: $0 current | next <patch|minor|major> | set <MAJOR.MINOR.PATCH> | git-sha" >&2
	exit 2
}

case "${1:-}" in
current)
	tr -d '[:space:]' <"$file"
	echo
	;;
next)
	IFS=. read -r major minor patch <<<"$(tr -d '[:space:]' <"$file")"
	case "${2:-}" in
	patch) patch=$((patch + 1)) ;;
	minor) minor=$((minor + 1)) patch=0 ;;
	major) major=$((major + 1)) minor=0 patch=0 ;;
	*) usage ;;
	esac
	echo "$major.$minor.$patch"
	;;
set)
	[[ "${2:-}" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || usage
	echo "$2" >"$file"
	echo "$2"
	;;
git-sha)
	sha="$(git -C "$root" rev-parse --short HEAD 2>/dev/null || echo unknown)"
	# .version and docs/ do not make a build dirty.
	if [[ -n "$(git -C "$root" status --porcelain -- . ':!.version' ':!docs' 2>/dev/null)" ]]; then
		sha="$sha-dirty"
	fi
	echo "$sha"
	;;
*)
	usage
	;;
esac
