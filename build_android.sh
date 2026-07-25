#!/usr/bin/env bash
set -euo pipefail
# Build Weiss Android AAR (gomobile). Run from this repo root after: go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init
# Recent NDKs only ship platforms.json for API 21+; gomobile still defaults to API 16, so -androidapi 21 is required.
: "${ANDROID_HOME:?Set ANDROID_HOME to your Android SDK}"
: "${ANDROID_NDK_HOME:?Set ANDROID_NDK_HOME to an NDK dir under \$ANDROID_HOME/ndk/<version>}"
export PATH="$(go env GOPATH)/bin:${PATH}"
exec gomobile bind -target=android -androidapi 21 -o weiss.aar .
