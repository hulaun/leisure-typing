#!/usr/bin/env bash
#
# Build the Android APK and drop it in bin/ for sideloading.
#
#   ./restart-android.sh              test, build release, copy to bin/bt-<version>.apk
#   ./restart-android.sh --no-test    skip the unit tests
#   ./restart-android.sh --debug      build the debug APK instead
#   ./restart-android.sh --install    also adb install it, if a device is attached
#
# The counterpart to ./restart.sh, which does the same for the terminal binary.
#
# There is no Gradle wrapper jar in the repo, deliberately: a cached Gradle
# distribution is already on this machine and a checked-in binary blob is worth
# avoiding. This script finds the newest one. If none is present, install Gradle
# or open android/ in Android Studio once, which will fetch it.
set -euo pipefail

cd "$(dirname "$0")"
ROOT="$PWD"

RUN_TESTS=1
VARIANT=release
INSTALL=0
for arg in "$@"; do
    case "$arg" in
        --no-test)  RUN_TESTS=0 ;;
        --test)     RUN_TESTS=1 ;;
        --debug)    VARIANT=debug ;;
        --install)  INSTALL=1 ;;
        -h|--help)  sed -n '2,12p' "$0" | sed 's/^# \?//'; exit 0 ;;
        *)          echo "unknown flag: $arg" >&2; exit 2 ;;
    esac
done

# --- the toolchain ----------------------------------------------------------

if [ -z "${JAVA_HOME:-}" ]; then
    for candidate in "/c/Program Files/Java/jdk-17" \
                     "/c/Program Files/Android/Android Studio/jbr"; do
        [ -x "$candidate/bin/java" ] && export JAVA_HOME="$candidate" && break
    done
fi
[ -n "${JAVA_HOME:-}" ] || { echo "no JDK found; set JAVA_HOME" >&2; exit 1; }

if [ -z "${ANDROID_HOME:-}" ]; then
    export ANDROID_HOME="$LOCALAPPDATA/Android/Sdk"
    [ -d "$ANDROID_HOME" ] || export ANDROID_HOME="$HOME/AppData/Local/Android/Sdk"
fi
[ -d "$ANDROID_HOME" ] || { echo "no Android SDK found; set ANDROID_HOME" >&2; exit 1; }

# The Gradle launcher: the wrapper if the project has one, else the newest
# cached distribution, else whatever is on PATH.
GRADLE=""
if [ -x "android/gradlew" ]; then
    GRADLE="./gradlew"
else
    DISTS="$HOME/.gradle/wrapper/dists"
    if [ -d "$DISTS" ]; then
        GRADLE=$(find "$DISTS" -type f -name gradle -path '*/bin/gradle' 2>/dev/null \
            | sort -V | tail -1)
    fi
    [ -n "$GRADLE" ] || GRADLE=$(command -v gradle || true)
fi
[ -n "$GRADLE" ] || {
    echo "no Gradle found. Open android/ in Android Studio once, or install Gradle." >&2
    exit 1
}

echo "java   $("$JAVA_HOME/bin/java" -version 2>&1 | head -1)"
echo "sdk    $ANDROID_HOME"
echo "gradle $GRADLE"
echo

cd "$ROOT/android"

# --- build ------------------------------------------------------------------

if [ "$RUN_TESTS" = 1 ]; then
    # The conformance test is the one that matters: it checks the Kotlin
    # wrapper still agrees with the Go one record for record. If the Go side
    # changed deliberately, regenerate the golden first:
    #   LEISURE_GOLDEN=1 go test ./internal/wrap/
    echo "test..."
    "$GRADLE" --no-daemon testDebugUnitTest
fi

echo "build $VARIANT..."
if [ "$VARIANT" = debug ]; then
    "$GRADLE" --no-daemon assembleDebug
    APK="app/build/outputs/apk/debug/app-debug.apk"
else
    "$GRADLE" --no-daemon assembleRelease
    APK="app/build/outputs/apk/release/app-release.apk"
fi
[ -f "$APK" ] || { echo "no APK at $APK" >&2; exit 1; }

VERSION=$(grep -o 'versionName = "[^"]*"' app/build.gradle.kts | sed 's/.*"\(.*\)"/\1/')
# Built artefacts live in bin/, beside bt.exe, rather than loose at the repo
# root: one gitignored directory holds everything either restart script builds.
mkdir -p "$ROOT/bin"
OUT="$ROOT/bin/bt-${VERSION:-0}${VARIANT:+-$VARIANT}.apk"
[ "$VARIANT" = release ] && OUT="$ROOT/bin/bt-${VERSION:-0}.apk"
cp "$APK" "$OUT"

echo
echo "built $(basename "$OUT")  ($(du -h "$OUT" | cut -f1))"
echo "  $OUT"

# --- install ----------------------------------------------------------------

if [ "$INSTALL" = 1 ]; then
    if [ -z "$(adb devices | sed -n '2,$p' | grep -w device || true)" ]; then
        echo
        echo "no device attached; copy the APK across and install it there."
        exit 0
    fi
    echo
    echo "installing..."
    adb install -r "$OUT"
fi
