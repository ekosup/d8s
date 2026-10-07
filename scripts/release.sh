#!/usr/bin/env bash
# Cuts and publishes a release in one go.
#
#   scripts/release.sh status              what is unreleased, and what would come next
#   scripts/release.sh patch|minor|major   bump the version, then release it
#   scripts/release.sh X.Y.Z               release exactly that version
#   scripts/release.sh current             release the version in .version; if it is
#                                          already tagged, publish that tag as it stands
#
# Add --dry-run to see every step without changing anything, and --yes to
# skip the confirmation. GITHUB_TOKEN is read from the environment or .env,
# and so is the optional HOMEBREW_TAP_TOKEN for the Homebrew tap.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

dry_run=0 assume_yes=0 what=""
for arg in "$@"; do
	case "$arg" in
	--dry-run) dry_run=1 ;;
	--yes) assume_yes=1 ;;
	*) what="$arg" ;;
	esac
done

die() {
	echo "release: $*" >&2
	exit 1
}

last_tag="$(git describe --tags --abbrev=0 2>/dev/null || true)"
current="$(scripts/version.sh current)"
range="${last_tag:+$last_tag..}HEAD"

# unreleased lists the commits since the last tag that change what users
# run. Tooling, tests and documentation do not call for a release.
unreleased() { git log --oneline --no-decorate "$range" -- cmd internal go.mod go.sum ':!*_test.go'; }

# suggest proposes the size of the next release from the commit subjects.
suggest() {
	if unreleased | grep -qE '^[0-9a-f]+ feat'; then echo minor; else echo patch; fi
}

# published reports whether a tag has been released: yes, no, or unknown
# when GitHub cannot be reached. It asks for the checksum file every release
# carries, which needs neither a token nor the rate-limited API.
published() {
	local code
	code="$(curl -sL -o /dev/null -m 10 -w '%{http_code}' "https://github.com/ekosup/d8s/releases/download/$1/checksums.txt" 2>/dev/null || true)"
	case "$code" in
	200) echo yes ;;
	404) echo no ;;
	*) echo unknown ;;
	esac
}

status() {
	local count part
	count="$(unreleased | wc -l | tr -d ' ')"
	echo "version file:  $current"
	if [[ -z "$last_tag" ]]; then
		echo "last tag:      none"
	else
		echo "last tag:      $last_tag (published on GitHub: $(published "$last_tag"))"
	fi
	if ! git rev-parse -q --verify "refs/tags/v$current" >/dev/null; then
		echo "next:          make release-next PART=current   (v$current is not tagged yet)"
		return
	fi
	if [[ "$(published "v$current")" == "no" ]]; then
		echo "next:          make release-next PART=current   (v$current is tagged but not published)"
	fi
	if [[ "$count" == 0 ]]; then
		echo "unreleased:    nothing since $last_tag"
		return
	fi
	echo "unreleased:    $count commit(s) since $last_tag that change the program"
	unreleased | head -15 | sed 's/^/  /'
	[[ "$count" -gt 15 ]] && echo "  ... and $((count - 15)) more"
	part="$(suggest)"
	echo "then:          make release-next PART=$part   (v$(scripts/version.sh next "$part"))"
}

load_token() {
	if [[ -z "${GITHUB_TOKEN:-}" && -f .env ]]; then
		GITHUB_TOKEN="$(sed -n 's/^GITHUB_TOKEN=//p' .env | tr -d '[:space:]"'"'"'')"
	fi
	if [[ -z "${GITHUB_TOKEN:-}" ]]; then
		[[ "$dry_run" == 1 ]] && { echo "note: GITHUB_TOKEN is empty; a real run would stop here, as a line GITHUB_TOKEN=<token>"; return; }
		die "GITHUB_TOKEN is empty; put it in .env as a line GITHUB_TOKEN=<token>"
	fi
	export GITHUB_TOKEN
	if [[ -z "${HOMEBREW_TAP_TOKEN:-}" && -f .env ]]; then
		HOMEBREW_TAP_TOKEN="$(sed -n 's/^HOMEBREW_TAP_TOKEN=//p' .env | tr -d '[:space:]"'"'"'')"
	fi
	export HOMEBREW_TAP_TOKEN="${HOMEBREW_TAP_TOKEN:-}"
}

confirm() {
	[[ "$assume_yes" == 1 ]] && return
	local answer
	[[ -t 0 ]] || die "no terminal to confirm on; nothing was changed. Run it again with YES=1 (or --yes)"
	read -r -p "publish $1 to GitHub? [y/N] " answer
	[[ "$answer" == "y" || "$answer" == "Y" ]] || die "cancelled"
}

# publish_tag builds and publishes an existing tag exactly as it was
# tagged, from a temporary checkout, whatever the working tree looks like.
publish_tag() {
	local tag="$1" version="${1#v}" work
	[[ "$(published "$tag")" == "yes" ]] && die "$tag is already published"
	load_token
	echo "publishing existing tag $tag ($(git rev-parse --short "$tag^{commit}"))"
	if [[ "$dry_run" == 1 ]]; then
		echo "dry run: would check out $tag in a temporary directory, build it and publish"
		echo "         it to GitHub Releases. Nothing was changed."
		return
	fi
	confirm "$tag"
	work="$(mktemp -d)"
	trap 'git worktree remove --force "$work" >/dev/null 2>&1 || true' EXIT
	git worktree add -q --detach "$work" "$tag"
	(cd "$work" && D8S_VERSION="$version" "$root/bin/tools/goreleaser" release --clean --config "$root/.goreleaser.yaml")
	echo "published: https://github.com/ekosup/d8s/releases/tag/$tag"
}

case "$what" in
"" | status)
	status
	exit 0
	;;
current) version="$current" ;;
patch | minor | major) version="$(scripts/version.sh next "$what")" ;;
*)
	[[ "$what" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || die "unknown argument '$what'; see the top of scripts/release.sh"
	version="$what"
	;;
esac
tag="v$version"

[[ -x bin/tools/goreleaser ]] || die "goreleaser is missing; run 'make tools'"

if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
	[[ "$what" == "current" ]] || die "$tag already exists"
	publish_tag "$tag"
	exit 0
fi

# Everything that could stop a release is checked before anything changes.
[[ "$(git rev-parse --abbrev-ref HEAD)" == "main" ]] || die "releases are cut from main"
[[ -z "$(git status --porcelain)" ]] || die "the working tree has uncommitted changes"
git fetch -q origin main || die "cannot reach origin"
[[ -z "$(git log --oneline HEAD..origin/main)" ]] || die "origin/main has commits you do not have; pull first"
load_token

echo "releasing $tag (previous: ${last_tag:-none})"
unreleased | head -20 | sed 's/^/  /'

if [[ "$dry_run" == 1 ]]; then
	echo "dry run: would run tests and lint, set .version to $version, commit, tag $tag,"
	echo "         push main and the tag, and publish to GitHub Releases. Nothing was changed."
	exit 0
fi
confirm "$tag"

make --no-print-directory test lint

if [[ "$version" != "$current" ]]; then
	scripts/version.sh set "$version" >/dev/null
	git commit -q -m "chores: release $tag" .version
fi
git tag -a "$tag" -m "$tag"
git push -q origin main "$tag"

D8S_VERSION="$version" bin/tools/goreleaser release --clean
echo "published: https://github.com/ekosup/d8s/releases/tag/$tag"
