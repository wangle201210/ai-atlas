#!/bin/sh
# Requires a Developer ID Application identity and an existing notarytool keychain profile.
set -eu
: "${AI_ATLAS_SIGN_IDENTITY:?Set a Developer ID Application identity}"
: "${AI_ATLAS_NOTARY_PROFILE:?Set an existing notarytool keychain profile}"
case "$AI_ATLAS_SIGN_IDENTITY" in
  'Developer ID Application:'*) ;;
  *) echo 'A Developer ID Application identity is required; development/ad-hoc identities are not accepted.' >&2; exit 1;;
esac
cd "$(dirname "$0")/.."
test -d bin/ai-atlas.app
mkdir -p release-dist
codesign --force --options runtime --timestamp --sign "$AI_ATLAS_SIGN_IDENTITY" bin/ai-atlas.app
codesign --verify --deep --strict bin/ai-atlas.app
ditto -c -k --keepParent bin/ai-atlas.app release-dist/notarization.zip
xcrun notarytool submit release-dist/notarization.zip --keychain-profile "$AI_ATLAS_NOTARY_PROFILE" --wait --output-format json > release-dist/notarization-result.json
python3 -c 'import json; r=json.load(open("release-dist/notarization-result.json")); assert r.get("status")=="Accepted", r'
xcrun stapler staple bin/ai-atlas.app
xcrun stapler validate bin/ai-atlas.app
printf '%s\n' 'Signed and notarized. Run package-release.py to create the final update archive.'
