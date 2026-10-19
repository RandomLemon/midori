#!/bin/sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
VERSION=${VERSION:-0.1.0}
APP_ID=io.github.randomlemon.midori
case "$VERSION" in
"" | *[!0-9A-Za-z.+-]*)
	echo "package: VERSION contains unsupported characters" >&2
	exit 2
	;;
esac
ARCH=${GOARCH:-$(go env GOARCH)}
BUNDLE="midori-$VERSION-linux-$ARCH"
DIST="$ROOT/dist"
STAGE="$DIST/$BUNDLE"
BINARY="$STAGE/usr/local/bin/midori"

rm -rf "$STAGE"
rm -f "$DIST/$BUNDLE.tar.xz"
mkdir -p "$STAGE/usr/local/bin" \
	"$STAGE/usr/local/share/applications" \
	"$STAGE/usr/local/share/icons/hicolor/512x512/apps"

GOOS=linux CGO_ENABLED=1 go build -tags wayland -trimpath \
	-ldflags "-s -w -X main.version=$VERSION" \
	-o "$BINARY" "$ROOT/cmd/midori"

version_output=$("$BINARY" --version)
if [ "$version_output" != "midori $VERSION" ]; then
	echo "packaging: version check failed: got '$version_output', want 'midori $VERSION'" >&2
	exit 1
fi

install -m 0644 "$ROOT/assets/icon.png" \
	"$STAGE/usr/local/share/icons/hicolor/512x512/apps/$APP_ID.png"
desktop_dir=$(mktemp -d)
trap 'rm -rf "$desktop_dir"' EXIT
desktop="$desktop_dir/$APP_ID.desktop"
sed \
	-e "s|^Exec=.*|Exec=/usr/local/bin/midori|" \
	-e "s|^TryExec=.*|TryExec=/usr/local/bin/midori|" \
	"$ROOT/packaging/$APP_ID.desktop" >"$desktop"
install -m 0644 "$desktop" "$STAGE/usr/local/share/applications/$APP_ID.desktop"

if command -v desktop-file-validate >/dev/null 2>&1; then
	desktop-file-validate "$STAGE/usr/local/share/applications/$APP_ID.desktop"
fi

tar -C "$DIST" -cJf "$DIST/$BUNDLE.tar.xz" "$BUNDLE"
echo "$DIST/$BUNDLE.tar.xz"
