#!/usr/bin/env bash
#
# Rebuild bt and start it again.
#
# This is the inner loop while working on the Go code, and it exists because
# the obvious way round is quietly wrong on Windows: a running bt.exe holds a
# lock on the file, so `go build -o bin/bt.exe` does not fail — it renames the
# old binary to bin/bt.exe~ and writes the new one, while the copy you are
# looking at goes on running the old code. The symptom is a change that has no
# effect, tests and all green, with nothing to say why.
#
# It differs from quick-tools' restart.sh in one deliberate way, and the reason
# is worth keeping. That script kills the running copy first, because the
# palette hides rather than closes and there is no clean way to end it. bt is
# the opposite: Esc quits it, saves the reading position, restores the console
# mode and drops the alternate screen buffer. Killing it instead skips every
# one of those — the defers never run, so the terminal it was reading in is
# left in raw mode with no echo, which looks like the shell is broken rather
# than like anything to do with this app (CLAUDE.md, gotchas 4 and 7). So the
# default is to refuse and say so. --force is there when the copy is a
# forgotten one in a terminal you have already lost.
#
# Usage:
#   ./restart.sh              rebuild and run
#   ./restart.sh --test       go vet and go test first
#   ./restart.sh --no-launch  rebuild only, do not start it
#   ./restart.sh --force      kill a running copy instead of refusing

set -euo pipefail

cd "$(dirname "$0")"
export PATH="$PATH:/c/Program Files/Go/bin"

EXE=bt.exe
OUT=bin/$EXE
SHIM="$LOCALAPPDATA/Microsoft/WindowsApps/bt.cmd"
test=0
launch=1
force=0

for arg in "$@"; do
	case "$arg" in
	--test) test=1 ;;
	--no-launch) launch=0 ;;
	--force) force=1 ;;
	*)
		echo "unknown option: $arg" >&2
		echo "usage: $0 [--test] [--no-launch] [--force]" >&2
		exit 2
		;;
	esac
done

running() { tasklist //FI "IMAGENAME eq $EXE" 2>/dev/null | grep -q "$EXE"; }

# --- 1. make sure nothing holds the exe ----------------------------------
if running; then
	if [ "$force" = 0 ]; then
		echo "bt is already running." >&2
		echo >&2
		echo "Press Esc in that window to quit it: that saves your place in the" >&2
		echo "book and puts the terminal back the way it was. Then run this again." >&2
		echo >&2
		echo "If that window is gone and only the process is left, --force kills it." >&2
		exit 1
	fi

	echo "force: killing the running copy"
	taskkill //F //IM "$EXE" >/dev/null 2>&1 || true

	# taskkill returns before the kernel has released the file handle, and
	# building into that window is what produces bin/bt.exe~.
	for _ in $(seq 20); do
		running || break
		sleep 0.1
	done

	echo "force: it was killed, not quit, so up to two seconds of typing since" >&2
	echo "       the last save is gone, and the terminal it was reading in is" >&2
	echo "       still in raw mode — close it, or run 'reset'." >&2
fi

# The renamed-out-of-the-way copy from a build that ran while bt was up.
rm -f "$OUT~"

# --- 2. build ------------------------------------------------------------
if [ "$test" = 1 ]; then
	echo "vet..."
	go vet ./...
	echo "test..."
	go test ./...
fi

echo "build..."
mkdir -p "$(dirname "$OUT")"
go build -o "$OUT" ./cmd/bt
echo "built $OUT"

# --- 3. the shim ---------------------------------------------------------
# %LOCALAPPDATA%\Microsoft\WindowsApps is on the Windows PATH by default, so a
# one-line .cmd there is what makes `bt` work from cmd, PowerShell, Git Bash
# and the VS Code terminal, from any directory. Written if it is missing;
# never overwritten, in case it has been pointed somewhere on purpose.
target="$(cygpath -w "$PWD/$OUT")"
if [ ! -f "$SHIM" ]; then
	printf '@echo off\r\n"%s" %%*\r\n' "$target" >"$SHIM"
	echo "shim: wrote $SHIM"
elif ! grep -qiF "$target" "$SHIM"; then
	echo "shim: $SHIM points somewhere else; leaving it alone" >&2
fi

# --- 4. run --------------------------------------------------------------
if [ "$launch" = 0 ]; then
	echo "not launching (--no-launch)"
	exit 0
fi

# bt owns the whole terminal while it runs, so it goes in this one — there is
# nothing to detach. Under mintty stdin is a pipe rather than a console handle
# and raw mode cannot be set on it, which winpty exists to fix.
if [ "${MSYSCON:-}" = "mintty.exe" ] && command -v winpty >/dev/null 2>&1; then
	exec winpty "$OUT"
fi
exec "$OUT"
