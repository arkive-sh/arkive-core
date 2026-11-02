#!/usr/bin/env bash
set -euo pipefail

WEB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK_DIR="$WEB_DIR/.heic-build"
BUILD_DIR="$WORK_DIR/build/libheif-1.23.7-libde265-1.1.3"
OUTPUT_DIR="$WEB_DIR/static/wasm/heic"

LIBHEIF_VERSION="1.23.7"
LIBHEIF_COMMIT="12a259a5882dd99d45ec5fc77e296a3b620d1e2e"
LIBDE265_VERSION="1.1.3"
LIBDE265_COMMIT="ba62bf4cfb3242f3bf0a45617ff09e35236e4d82"
CORES="${CORES:-$(getconf _NPROCESSORS_ONLN)}"

for command in git emcc emcmake emmake emconfigure; do
	command -v "$command" >/dev/null || { echo "Missing required build tool: $command" >&2; exit 1; }
done

# libheif's build helper assumes the emsdk directory layout even with system Emscripten.
EMSDK_SHIM_DIR=""
if [[ -z "${EMSDK:-}" || ! -x "$EMSDK/upstream/bin/llvm-nm" ]]; then
	LLVM_NM="$(command -v llvm-nm || command -v emnm || true)"
	[[ -n "$LLVM_NM" ]] || { echo "Missing llvm-nm from Emscripten." >&2; exit 1; }
	EMSDK_SHIM_DIR="$(mktemp -d "${TMPDIR:-/tmp}/arkive-emsdk.XXXXXX")"
	mkdir -p "$EMSDK_SHIM_DIR/upstream/bin"
	ln -s "$LLVM_NM" "$EMSDK_SHIM_DIR/upstream/bin/llvm-nm"
	EMSDK="$EMSDK_SHIM_DIR"
	export EMSDK
	trap 'rm -rf "$EMSDK_SHIM_DIR"' EXIT
fi

mkdir -p "$WORK_DIR/sources" "$BUILD_DIR" "$OUTPUT_DIR"

clone_pinned() {
	local name="$1" version="$2" commit="$3" repository="$4"
	local path="$WORK_DIR/sources/$name-$version"
	if [[ ! -d "$path/.git" ]]; then
		git clone --depth 1 --branch "$version" "$repository" "$path"
	fi
	local actual
	actual="$(git -C "$path" rev-parse HEAD)"
	if [[ "$actual" != "$commit" ]]; then
		echo "$name $version resolved to $actual; expected pinned commit $commit." >&2
		exit 1
	fi
	printf '%s\n' "$path"
}

LIBDE265_SOURCE="$(clone_pinned "libde265" "v$LIBDE265_VERSION" "$LIBDE265_COMMIT" https://github.com/strukturag/libde265.git)"
LIBHEIF_SOURCE="$(clone_pinned "libheif" "v$LIBHEIF_VERSION" "$LIBHEIF_COMMIT" https://github.com/strukturag/libheif.git)"

LIBDE265_BUILD_DIR="$BUILD_DIR/build-libde265-$LIBDE265_VERSION"
if [[ ! -s "$LIBDE265_SOURCE/libde265/.libs/libde265.a" ]]; then
	emcmake cmake -S "$LIBDE265_SOURCE" -B "$LIBDE265_BUILD_DIR" \
		-DCMAKE_BUILD_TYPE=Release \
		-DCMAKE_ARCHIVE_OUTPUT_DIRECTORY="$LIBDE265_SOURCE/libde265/.libs" \
		-DBUILD_SHARED_LIBS=OFF -DENABLE_SDL=OFF -DENABLE_SIMD=OFF \
		-DENABLE_DECODER=OFF \
		-DENABLE_ENCODER=OFF -DENABLE_SHERLOCK265=OFF \
		-DENABLE_INTERNAL_DEVELOPMENT_TOOLS=OFF -DWITH_FUZZERS=OFF
	emmake cmake --build "$LIBDE265_BUILD_DIR" --parallel "$CORES"
fi
# The upstream libheif script expects libde265's generated public header in its source include tree.
cp "$LIBDE265_BUILD_DIR/libde265/de265-version.h" "$LIBDE265_SOURCE/libde265/de265-version.h"
ln -sfn "$LIBDE265_SOURCE" "$BUILD_DIR/libde265-$LIBDE265_VERSION"

BUILD_SCRIPT="$BUILD_DIR/build-emscripten.sh"
cp "$LIBHEIF_SOURCE/build-emscripten.sh" "$BUILD_SCRIPT"
# The Emscripten build cannot link libheif's pthread-dependent tests.
sed -i 's/-DENABLE_PLUGIN_LOADING=OFF"/-DENABLE_PLUGIN_LOADING=OFF -DBUILD_TESTING=OFF"/' "$BUILD_SCRIPT"
# Link the C++ static libraries with Emscripten's C++ driver.
sed -i 's/^emcc -Wl,--whole-archive/em++ -Wl,--whole-archive/' "$BUILD_SCRIPT"

(
	cd "$BUILD_DIR"
	LIBDE265_VERSION="$LIBDE265_VERSION" \
	ENABLE_LIBDE265=1 ENABLE_AOM=0 ENABLE_UNCOMPRESSED=0 ENABLE_OPENJPEG=0 ENABLE_WEBCODECS=0 \
	USE_ES6=1 USE_WASM=1 USE_TYPESCRIPT=0 USE_UNSAFE_EVAL=0 CORES="$CORES" \
	"$BUILD_SCRIPT" "$LIBHEIF_SOURCE"
)

cp "$BUILD_DIR/libheif.js" "$OUTPUT_DIR/libheif.js"
cp "$BUILD_DIR/libheif.wasm" "$OUTPUT_DIR/libheif.wasm"
cp "$LIBHEIF_SOURCE/COPYING" "$OUTPUT_DIR/libheif-LICENSE"
cp "$LIBDE265_SOURCE/COPYING" "$OUTPUT_DIR/libde265-LICENSE"
cat > "$OUTPUT_DIR/build-info.txt" <<EOF
libheif $LIBHEIF_VERSION ($LIBHEIF_COMMIT)
libde265 $LIBDE265_VERSION ($LIBDE265_COMMIT)
Only libde265 HEVC decoding is enabled. Encoders, other decoders, plugins, and WebCodecs are disabled.
EOF

echo "Built pinned HEIC decoder assets in $OUTPUT_DIR"
