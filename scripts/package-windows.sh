#!/usr/bin/env bash
# Package Muse for Windows x64 as a portable folder + zip that a friend can
# simply extract and double-click.
#
#   Muse.exe            the app (built here, cross-compiled for windows/amd64)
#   ffmpeg.exe          bundled so thumbnails/metadata work without installing
#   ffprobe.exe         anything (see backend/platform/tools.go: exe dir first)
#   README.txt          first-run notes for non-technical users
#   FFmpeg-LICENSE.txt  LGPL license text (required when redistributing)
#
# Usage:
#   scripts/package-windows.sh
#   MUSE_SKIP_BUILD=1 scripts/package-windows.sh   # reuse build/bin/Muse.exe
#   MUSE_NO_FFMPEG=1 scripts/package-windows.sh    # ship without FFmpeg
#   MUSE_FFMPEG_FLAVOR=static scripts/package-windows.sh  # 2 big exes, ~130 MB zip
#   MUSE_FFMPEG_ZIP=/path/to/ffmpeg-...-win64-lgpl.zip scripts/package-windows.sh
#
# Output: dist/Muse-<version>-win64/ and dist/Muse-<version>-win64.zip
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

die() { printf 'error: %s\n' "$*" >&2; exit 1; }
step() { printf '\n==> %s\n' "$*"; }

# FFmpeg comes from BtbN's LGPL builds. "shared" ships ffmpeg.exe + ffprobe.exe
# plus their DLLs (~65 MB zip); "static" is two self-contained 130 MB exes
# (~130 MB zip). Shared is the default because friends have to download it.
FFMPEG_BASE="https://github.com/BtbN/FFmpeg-Builds/releases/latest/download"
FFMPEG_FLAVOR="${MUSE_FFMPEG_FLAVOR:-shared}"
case "$FFMPEG_FLAVOR" in
    shared) FFMPEG_ASSET="ffmpeg-master-latest-win64-lgpl-shared.zip" ;;
    static) FFMPEG_ASSET="ffmpeg-master-latest-win64-lgpl.zip" ;;
    *) die "MUSE_FFMPEG_FLAVOR must be 'shared' or 'static', got '$FFMPEG_FLAVOR'" ;;
esac
CACHE_DIR="$ROOT/.cache/ffmpeg-win64-$FFMPEG_FLAVOR"

# ---------- toolchain ----------

command -v go >/dev/null 2>&1 || die "go not found in PATH"
command -v node >/dev/null 2>&1 || die "node not found in PATH (needed for the frontend build)"
command -v npm >/dev/null 2>&1 || die "npm not found in PATH"

WAILS="$(command -v wails || true)"
if [ -z "$WAILS" ] && [ -x "${HOME}/go/bin/wails" ]; then
    WAILS="${HOME}/go/bin/wails"
fi
[ -n "$WAILS" ] || die "wails CLI not found. Install it with:
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.15.0"

# Version comes from wails.json so the exe metadata, the folder name and the zip
# name always agree. Bump it there, not here.
VERSION="$(node -e 'const c=require("./wails.json");process.stdout.write((c.info&&c.info.productVersion)||"1.0.0")')"
[ -n "$VERSION" ] || die "cannot read info.productVersion from wails.json"

STAGE="$ROOT/dist/Muse-${VERSION}-win64"
ZIP="$ROOT/dist/Muse-${VERSION}-win64.zip"

printf 'Muse %s — portable Windows x64 package\n' "$VERSION"
printf '  wails: %s\n' "$WAILS"
printf '  go:    %s\n' "$(go version | awk '{print $3}')"

# ---------- build ----------

if [ "${MUSE_SKIP_BUILD:-0}" = "1" ] && [ -f build/bin/Muse.exe ]; then
    step "Reusing build/bin/Muse.exe"
else
    step "Building build/bin/Muse.exe (windows/amd64)"
    "$WAILS" build -platform windows/amd64 -clean -ldflags "-s -w"
fi

[ -f build/bin/Muse.exe ] || die "build/bin/Muse.exe was not produced"
file build/bin/Muse.exe | grep -q 'PE32+' || die "build/bin/Muse.exe is not a 64-bit Windows executable"

# ---------- bundled FFmpeg ----------

FFMPEG_FILES=()

if [ "${MUSE_NO_FFMPEG:-0}" = "1" ]; then
    step "Skipping FFmpeg (MUSE_NO_FFMPEG=1)"
else
    step "Preparing bundled FFmpeg (LGPL build)"

    ZIP_SRC="${MUSE_FFMPEG_ZIP:-$CACHE_DIR/$FFMPEG_ASSET}"
    if [ ! -f "$ZIP_SRC" ]; then
        command -v curl >/dev/null 2>&1 || die "curl not found and $ZIP_SRC is missing"
        mkdir -p "$CACHE_DIR"
        printf '  downloading %s (~170 MB, cached in .cache/)\n' "$FFMPEG_ASSET"
        curl -fSL --retry 3 -o "$ZIP_SRC.part" "$FFMPEG_BASE/$FFMPEG_ASSET"
        mv "$ZIP_SRC.part" "$ZIP_SRC"
    else
        printf '  using cached %s\n' "$ZIP_SRC"
    fi

    EXTRACT_DIR="$CACHE_DIR/extracted"
    rm -rf "$EXTRACT_DIR"
    mkdir -p "$EXTRACT_DIR"
    command -v unzip >/dev/null 2>&1 || die "unzip not found"
    if [ "$FFMPEG_FLAVOR" = "shared" ]; then
        # ffmpeg.exe/ffprobe.exe load avcodec/avformat/... DLLs from their own
        # folder, so every DLL next to them must ship. ffplay is unused.
        unzip -q -j "$ZIP_SRC" '*/bin/*' '*/LICENSE.txt' -d "$EXTRACT_DIR"
        rm -f "$EXTRACT_DIR/ffplay.exe"
    else
        unzip -q -j "$ZIP_SRC" '*/bin/ffmpeg.exe' '*/bin/ffprobe.exe' '*/LICENSE.txt' -d "$EXTRACT_DIR"
    fi

    [ -f "$EXTRACT_DIR/ffmpeg.exe" ] || die "ffmpeg.exe not found inside $ZIP_SRC"
    [ -f "$EXTRACT_DIR/ffprobe.exe" ] || die "ffprobe.exe not found inside $ZIP_SRC"
    FFMPEG_FILES=()
    while IFS= read -r f; do FFMPEG_FILES+=("$f"); done < <(find "$EXTRACT_DIR" -maxdepth 1 -type f ! -name LICENSE.txt)
fi

# ---------- stage ----------

step "Staging $STAGE"
rm -rf "$STAGE"
mkdir -p "$STAGE"

cp build/bin/Muse.exe "$STAGE/Muse.exe"
if [ "${#FFMPEG_FILES[@]}" -gt 0 ]; then
    cp "${FFMPEG_FILES[@]}" "$STAGE/"
    cp "$EXTRACT_DIR/LICENSE.txt" "$STAGE/FFmpeg-LICENSE.txt"
fi
cp packaging/README-Windows.txt "$STAGE/README.txt"
cp packaging/THIRD-PARTY-NOTICES.txt "$STAGE/THIRD-PARTY-NOTICES.txt"

# Windows users should be able to read these in Notepad; LF-only text files are
# fine on Windows 10 1903+ but CRLF removes any doubt.
if command -v perl >/dev/null 2>&1; then
    perl -pi -e 's/\r?\n/\r\n/' "$STAGE/README.txt" "$STAGE/THIRD-PARTY-NOTICES.txt"
fi

# ---------- zip ----------

step "Creating $(basename "$ZIP")"
rm -f "$ZIP"
if command -v zip >/dev/null 2>&1; then
    ( cd "$ROOT/dist" && zip -q -r -9 "$(basename "$ZIP")" "$(basename "$STAGE")" )
elif command -v 7z >/dev/null 2>&1; then
    ( cd "$ROOT/dist" && 7z a -tzip -mx=9 "$(basename "$ZIP")" "$(basename "$STAGE")" >/dev/null )
elif command -v tar >/dev/null 2>&1; then
    ( cd "$ROOT/dist" && tar -a -c -f "$(basename "$ZIP")" "$(basename "$STAGE")" )
else
    die "no zip, 7z or zip-capable tar found"
fi

# ---------- report ----------

if command -v sha256sum >/dev/null 2>&1; then
    SHA="$(sha256sum "$ZIP" | awk '{print $1}')"
else
    SHA="$(shasum -a 256 "$ZIP" | awk '{print $1}')"
fi

printf '\nDone.\n'
printf '  folder: %s\n' "$STAGE"
printf '  zip:    %s (%.0f MB)\n' "$ZIP" "$(( $(wc -c < "$ZIP") / 1048576 ))"
printf '  sha256: %s\n' "$SHA"
printf '\nRe-send the zip after bumping info.productVersion in wails.json; friends\nreplace the folder contents and keep %%LOCALAPPDATA%%\\Muse\\ untouched.\n'
