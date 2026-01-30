#!/bin/bash

HELP_TEXT="calculate_next_version.sh -r <absolute_path_to_repo> -h <help text>";

while getopts "r:h" flag; do
    case ${flag} in
        r) REPOSITORY_PATH=${OPTARG}
            ;;
        h) echo $HELP_TEXT; exit 0;
            ;;
    esac
done

if [[ -z "${REPOSITORY_PATH}" ]]; then
        echo $HELP_TEXT >&2;
        exit 1;
fi

pushd "${REPOSITORY_PATH}" 1>&2 1>/dev/null;

CONVENTIONAL_COMMIT_PREFIX="(fix|feat|chore|docs|ci)(!)?"
CONVENTIONAL_COMMIT_REGEX="^($CONVENTIONAL_COMMIT_PREFIX:|Merge)"

SIMPLE_SEMVER_REGEX="[0-9]+.[0-9]+.[0-9]+"

IS_PATCH=false;
IS_MINOR=false;
IS_MAJOR=false;
git log --format="%h" | while read -r SHA; do
        SUMMARY=$(git show ${SHA} --format="%s" -s )
        echo "SHA: \"${SHA}\", SUMMARY: \"${SUMMARY}\"" >&2;

        if [[ $SUMMARY =~ $CONVENTIONAL_COMMIT_REGEX ]]; then
                echo "Commit matched pattern" >&2;
                PREFIX=$(echo $SUMMARY | grep -E -o "$CONVENTIONAL_COMMIT_PREFIX")
                echo "Conventional Commit Prefix: \"${PREFIX}\"" >&2;
                if [[ "$PREFIX" =~ "!" ]]; then
                        IS_MAJOR=true;
                elif [[ "$PREFIX" == "feat" ]]; then
                        IS_MINOR=true;
                else
                        IS_PATCH=true;
                fi
        else
                echo "Commit is not a conventional commit - exiting." >&2;
                exit 1;
        fi

        TAG=$(git show ${SHA} --format="%d" -s | grep -o -E "tag: (${SIMPLE_SEMVER_REGEX})" | grep -o -E "${SIMPLE_SEMVER_REGEX}")
        echo "TAG: ${TAG}" >&2;
        if [[ -z "$TAG" ]]; then
                continue
        else
                echo "IS_PATCH: $IS_PATCH" >&2;
                echo "IS_MINOR: $IS_MINOR" >&2;

                CUR_MAJOR=$(echo "$TAG" | sed -r 's/^([0-9]+)\.([0-9]+)\.([0-9]+).*/\1/');
                CUR_MINOR=$(echo "$TAG" | sed -r 's/^([0-9]+)\.([0-9]+)\.([0-9]+).*/\2/');
                CUR_PATCH=$(echo "$TAG" | sed -r 's/^([0-9]+)\.([0-9]+)\.([0-9]+).*/\3/');
                echo "$CUR_MAJOR" >&2;
                echo "$CUR_MINOR" >&2;
                echo "$CUR_PATCH" >&2;

                if [[ "$IS_MAJOR" == true ]]; then
                        NEXT_MAJOR=$(($CUR_MAJOR+1));
                        RESULT="$NEXT_MAJOR.0.0";
                elif [[ "$IS_MINOR" == true ]]; then
                        NEXT_MINOR=$(($CUR_MINOR+1));
                        RESULT="$CUR_MAJOR.$NEXT_MINOR.0"
                elif [[ "$IS_PATCH" == true ]]; then
                        NEXT_PATCH=$(($CUR_PATCH+1));
                        RESULT="$CUR_MAJOR.$CUR_MINOR.$NEXT_PATCH";
                fi

                echo "RESULT: $RESULT" >&2;
                echo "$RESULT"
                break;
        fi
done 

popd 1>&2 1>/dev/null;
