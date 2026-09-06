#!/usr/bin/env bash
# Report what gopls says about every tracked Go file, and fail if it says
# anything not written down in .github/lint-allow.txt.
#
# gopls rather than a linter of its own, because gopls is what an editor
# already runs here: what is green on a desk is green in CI, and there is no
# second rule set to keep in step. It reads no configuration -- the list below
# is this repository's, and every line of it needs a reason.
#
# `gopls check` exits 0 whether or not it found something, so the check is
# whether anything is left after the allowed lines are removed.
set -euo pipefail

out=$(gopls check "$@" 2>&1 || true)
[ -n "$out" ] || exit 0

# The paths gopls prints are absolute; make them repository-relative so an
# allow entry is not tied to where the checkout lives.
out=$(printf '%s\n' "$out" | sed "s|^$PWD/||")

left=$(printf '%s\n' "$out" | grep -v -F -f <(grep -v '^\s*#' .github/lint-allow.txt | grep -v '^\s*$') || true)
[ -n "$left" ] || exit 0

echo "$left"
exit 1
