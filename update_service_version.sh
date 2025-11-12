#!/bin/bash

set -e

# Function to display usage
usage() {
    echo "Usage: $0 -v|--version <version>" >&2
    echo "Example: $0 -v 0.2.0" >&2
    exit 1
}

# Parse command line arguments
VERSION=""
while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--version)
            VERSION="$2"
            shift 2
            ;;
        *)
            echo "Error: Unknown option $1" >&2
            usage
            ;;
    esac
done

# Check if version was provided
if [[ -z "$VERSION" ]]; then
    echo "Error: Version is required" >&2
    usage
fi

# Check if version.go exists
if [[ ! -f "version.go" ]]; then
    echo "Error: version.go not found in current directory" >&2
    exit 1
fi

echo "Updating service version to $VERSION..." >&2

# Update version.go with the new version
sed -i.bak "s/serviceVersion = \".*\"/serviceVersion = \"$VERSION\"/" version.go

# Remove backup file created by sed
rm -f version.go.bak

# Add the change to git
git add version.go

# Commit the change
git commit -m "chore: Update service version to $VERSION"

# Tag the commit
git tag "$VERSION"

# Push changes and tags to upstream
git push origin HEAD
git push origin "$VERSION"

echo "Successfully updated service version to $VERSION and pushed to upstream" >&2