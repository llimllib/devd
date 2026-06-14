#!/bin/bash
set -e

# Get the latest tag, default to v0.0.0 if none exists
LATEST_TAG=$(git tag -l 'v*' | sort -V | tail -1)
if [ -z "$LATEST_TAG" ]; then
    LATEST_TAG="v0.0.0"
fi

# Parse version components
VERSION=${LATEST_TAG#v}
MAJOR=$(echo "$VERSION" | cut -d. -f1)
MINOR=$(echo "$VERSION" | cut -d. -f2)
PATCH=$(echo "$VERSION" | cut -d. -f3)

echo "Current version: $LATEST_TAG"
echo ""
echo "What type of release?"
echo "  1) patch  (v$MAJOR.$MINOR.$((PATCH + 1)))"
echo "  2) minor  (v$MAJOR.$((MINOR + 1)).0)"
echo "  3) major  (v$((MAJOR + 1)).0.0)"
echo ""
read -rp "Select [1-3]: " CHOICE

case $CHOICE in
    1)
        NEW_VERSION="v$MAJOR.$MINOR.$((PATCH + 1))"
        ;;
    2)
        NEW_VERSION="v$MAJOR.$((MINOR + 1)).0"
        ;;
    3)
        NEW_VERSION="v$((MAJOR + 1)).0.0"
        ;;
    *)
        echo "Invalid choice"
        exit 1
        ;;
esac

echo ""
echo "Will create and push tag: $NEW_VERSION"
read -rp "Continue? [y/N]: " CONFIRM

if [ "$CONFIRM" != "y" ] && [ "$CONFIRM" != "Y" ]; then
    echo "Aborted"
    exit 1
fi

# Ensure we're on main branch and up to date
BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$BRANCH" != "main" ]; then
    echo "Warning: not on main branch (currently on $BRANCH)"
    read -rp "Continue anyway? [y/N]: " CONFIRM_BRANCH
    if [ "$CONFIRM_BRANCH" != "y" ] && [ "$CONFIRM_BRANCH" != "Y" ]; then
        echo "Aborted"
        exit 1
    fi
fi

# Check for uncommitted changes
if ! git diff-index --quiet HEAD --; then
    echo "Error: uncommitted changes exist"
    exit 1
fi

# Create and push tag
git tag -a "$NEW_VERSION" -m "Release $NEW_VERSION"
git push origin "$NEW_VERSION"

echo ""
echo "✓ Tagged and pushed $NEW_VERSION"
echo "  GitHub Actions will now build and publish the release"
echo "  Watch progress at: https://github.com/llimllib/devd/actions"
