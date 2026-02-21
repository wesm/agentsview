#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

VERSION="$1"
EXTRA_INSTRUCTIONS="${2:-}"

if [ -z "$VERSION" ]; then
    echo "Usage: $0 <version> [extra_instructions]"
    echo "Example: $0 0.2.0"
    echo "Example: $0 0.2.0 \"Focus on analytics improvements\""
    exit 1
fi

if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "Error: Version must be in format X.Y.Z (e.g., 0.2.0)"
    exit 1
fi

TAG="v$VERSION"

if git rev-parse "$TAG" >/dev/null 2>&1; then
    echo "Error: Tag $TAG already exists"
    exit 1
fi

if ! git diff-index --quiet HEAD --; then
    echo "Error: You have uncommitted changes. Please commit or stash them first."
    exit 1
fi

if ! command -v gh &> /dev/null; then
    echo "Error: gh CLI is required. Install from https://cli.github.com/"
    exit 1
fi

# Generate changelog
CHANGELOG_FILE=$(mktemp)
trap 'rm -f "$CHANGELOG_FILE"' EXIT

"$SCRIPT_DIR/changelog.sh" "$VERSION" "-" "$EXTRA_INSTRUCTIONS" > "$CHANGELOG_FILE"

echo ""
echo "=========================================="
echo "PROPOSED CHANGELOG FOR $TAG"
echo "=========================================="
cat "$CHANGELOG_FILE"
echo ""
echo "=========================================="
echo ""

read -p "Accept this changelog and create release $TAG? [y/N] " -n 1 -r
echo ""

if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Release cancelled."
    exit 0
fi

echo "Creating tag $TAG..."
git tag -a "$TAG" -m "Release $VERSION

$(cat $CHANGELOG_FILE)"

echo "Pushing tag to origin..."
git push origin "$TAG"

echo ""
echo "Release $TAG created and pushed successfully!"
echo "GitHub Actions will build and publish the release."
echo ""
echo "GitHub release URL: https://github.com/wesm/agentsview/releases/tag/$TAG"
