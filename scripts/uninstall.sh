#!/bin/sh
set -eu

APP_ID=io.github.randomlemon.midori
PREFIX=${PREFIX:-"$HOME/.local"}
DESTDIR=${DESTDIR:-}

rm -f "$DESTDIR$PREFIX/bin/midori" \
	"$DESTDIR$PREFIX/share/applications/$APP_ID.desktop" \
	"$DESTDIR$PREFIX/share/icons/hicolor/512x512/apps/$APP_ID.png"

if [ -z "$DESTDIR" ]; then
	if command -v update-desktop-database >/dev/null 2>&1; then
		update-desktop-database "$PREFIX/share/applications" >/dev/null 2>&1 || true
	fi
	if command -v gtk-update-icon-cache >/dev/null 2>&1; then
		gtk-update-icon-cache -q -t -f "$PREFIX/share/icons/hicolor" >/dev/null 2>&1 || true
	fi
fi

echo "Removed Midori from $DESTDIR$PREFIX"
