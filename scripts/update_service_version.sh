#!/bin/bash
HELP_TEXT="update_service_version.sh -h <help> -d <dry-run>"
DRY_RUN=false

while getopts "i:r:odh" flag; do
case ${flag} in
d) DRY_RUN=true
   ;;
h) echo $HELP_TEXT; exit 0;
   ;;
esac
done


REPOSITORY_PATH="."
NEXT_VERSION=$(./scripts/calculate_next_version.sh -r $REPOSITORY_PATH)
BRANCH_NAME=$(git branch --show-current)

# Check if version.go exists
if [[ ! -f "version.go" ]]; then
    echo "Error: version.go not found in current directory" >&2
    exit 1
fi

echo "Updating service version to $NEXT_VERSION..." >&2

# Update version.go with the new version
sed -i.bak "s/serviceVersion = \".*\"/serviceVersion = \"$NEXT_VERSION\"/" version.go

# Remove backup file created by sed
rm -f version.go.bak

if [ "$DRY_RUN" = true ]; then
    echo "[DRY RUN] Would execute: git commit --allow-empty -m \"ci: Release version $NEXT_VERSION\""
    echo "[DRY RUN] Would execute: git tag -a \"$NEXT_VERSION\" -m \"Release $NEXT_VERSION\""
    echo "[DRY RUN] Would execute: git push -u origin \"$BRANCH_NAME\""
    echo "[DRY RUN] Would execute: git push -u origin --tags"
else
    # Add the change to git
    git add version.go
    git commit --allow-empty -m "ci: Release version $NEXT_VERSION"
    git tag -a "$NEXT_VERSION" -m "Release $NEXT_VERSION"
    git push -u origin "$BRANCH_NAME"
    git push -u origin --tags
fi

echo "Successfully updated service version to $VERSION and pushed to upstream" >&2