#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
VERSION=${VERSION:-0.1.0}
case "$VERSION" in
"" | *[!0-9A-Za-z.+-]*)
	echo "install: VERSION contains unsupported characters" >&2
	exit 2
	;;
esac
APP_ID=io.github.randomlemon.midori
PREFIX=${PREFIX:-"$HOME/.local"}
DESTDIR=${DESTDIR:-}
build_dir=""
desktop_dir=""
cleanup() {
	if [ -n "$build_dir" ]; then rm -rf "$build_dir"; fi
	if [ -n "$desktop_dir" ]; then rm -rf "$desktop_dir"; fi
}
trap cleanup EXIT

if [ "${SKIP_BUILD:-0}" = "1" ]; then
	BINARY="$ROOT/bin/midori"
else
	build_dir=$(mktemp -d)
	BINARY="$build_dir/midori"
	GOOS=linux CGO_ENABLED=1 go build -tags wayland -trimpath \
		-ldflags "-X main.version=$VERSION" \
		-o "$BINARY" "$ROOT/cmd/midori"
fi
if [ ! -x "$BINARY" ]; then
	echo "install: missing $BINARY; run without SKIP_BUILD=1 or build it first" >&2
	exit 1
fi

install -D -m 0755 "$BINARY" "$DESTDIR$PREFIX/bin/midori"
install -D -m 0644 "$ROOT/assets/icon.png" \
	"$DESTDIR$PREFIX/share/icons/hicolor/512x512/apps/$APP_ID.png"

desktop_dir=$(mktemp -d)
desktop="$desktop_dir/$APP_ID.desktop"
sed \
	-e "s|^Exec=.*|Exec=$PREFIX/bin/midori|" \
	-e "s|^TryExec=.*|TryExec=$PREFIX/bin/midori|" \
	"$ROOT/packaging/$APP_ID.desktop" >"$desktop"
if command -v desktop-file-validate >/dev/null 2>&1; then
	desktop-file-validate "$desktop"
fi
install -D -m 0644 "$desktop" "$DESTDIR$PREFIX/share/applications/$APP_ID.desktop"

if [ -z "$DESTDIR" ]; then
	if command -v update-desktop-database >/dev/null 2>&1; then
		update-desktop-database "$PREFIX/share/applications" >/dev/null 2>&1 || true
	fi
	if command -v gtk-update-icon-cache >/dev/null 2>&1; then
		gtk-update-icon-cache -q -t -f "$PREFIX/share/icons/hicolor" >/dev/null 2>&1 || true
	fi
fi

echo "Installed Midori to $DESTDIR$PREFIX"
echo "Launch it from the application menu or as $PREFIX/bin/midori"
