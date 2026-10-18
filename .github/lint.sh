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
#
# It runs once per module, from that module's directory and with only its
# files. Handed the files of all four at once, gopls chose for itself which
# module's go.mod to load a file under, and now and then chose wrong: a test
# in the root module could not import a module only the root requires.
set -euo pipefail

root=$PWD

# The module a file belongs to is the nearest directory above it with a
# go.mod.
declare -A files
for f in "$@"; do
	dir=$(dirname "$f")
	while [ ! -f "$dir/go.mod" ]; do
		[ "$dir" != . ] || { echo "lint.sh: no go.mod above $f" >&2; exit 2; }
		dir=$(dirname "$dir")
	done
	files[$dir]+="${f#"$dir"/} "
done

out=
for mod in "${!files[@]}"; do
	# shellcheck disable=SC2086 # the list is split on purpose; paths have no spaces.
	out+=$(cd "$mod" && gopls check ${files[$mod]} 2>&1 || true)$'\n'
done
out=$(printf '%s' "$out" | sed '/^$/d')
[ -n "$out" ] || exit 0

# The paths gopls prints are absolute; make them repository-relative so an
# allow entry is not tied to where the checkout lives.
out=$(printf '%s\n' "$out" | sed "s|^$root/||")

left=$(printf '%s\n' "$out" | grep -v -F -f <(grep -v '^\s*#' .github/lint-allow.txt | grep -v '^\s*$') || true)
[ -n "$left" ] || exit 0

echo "$left"
exit 1
