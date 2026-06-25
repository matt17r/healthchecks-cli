#!/usr/bin/env bash
#
# Cut a release end-to-end:
#   1. goreleaser builds every OS/arch, creates the GitHub release, and uploads
#      the archives + checksums. It also writes the Homebrew cask into dist/ but
#      does NOT push it (skip_upload in .goreleaser.yaml).
#   2. `brew style --fix` autocorrects the generated cask — goreleaser's template
#      emits a stray blank line (Layout/EmptyLinesAroundBlockBody) and we want the
#      tap's `brew test-bot` CI to pass.
#   3. The fixed cask is committed and pushed to the Homebrew tap.
#
# Usage:
#   git tag vX.Y.Z && git push origin vX.Y.Z
#   scripts/release.sh                 # tag inferred from `git describe`
#   scripts/release.sh vX.Y.Z          # or pass it explicitly
#
# Requires: goreleaser, brew, gh (logged in with write access to both repos).
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

tag="${1:-$(git describe --tags --abbrev=0)}"
tap_repo="matt17r/homebrew-tap"
cask="dist/homebrew/Casks/hc.rb"

export GITHUB_TOKEN="${GITHUB_TOKEN:-$(gh auth token)}"

echo "==> Building and publishing $tag with goreleaser"
goreleaser release --clean

echo "==> Linting the generated cask (brew style --fix)"
brew style --fix "$cask"

echo "==> Pushing the cask to $tap_repo"
workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT
gh repo clone "$tap_repo" "$workdir" -- --depth 1 --quiet
mkdir -p "$workdir/Casks"
cp "$cask" "$workdir/Casks/hc.rb"
git -C "$workdir" add Casks/hc.rb
if git -C "$workdir" diff --cached --quiet; then
	echo "    Cask unchanged; nothing to push."
else
	git -C "$workdir" commit --quiet -m "hc $tag"
	git -C "$workdir" push --quiet origin HEAD
	echo "    Pushed Casks/hc.rb for $tag."
fi

echo "==> Released $tag"
