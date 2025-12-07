package version

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/rs/zerolog"
)

// Versioner holds state for calculating next semantic version
type Versioner struct {
}

// New creates a new Versioner for a given repo path
func NewVersioner() *Versioner {
	return &Versioner{}
}

// CalculateNextVersion walks commit history, checks conventional commits,
// finds the last tag, and returns the next semantic version.
func (v *Versioner) CalculateNextVersion(ctx context.Context, repoPath string) (*semver.Version, error) {
	log := zerolog.Ctx(ctx)
	log.Info().Msg("calculating next version")
	var isMajor, isMinor, isPatch bool

	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open repo: %w", err)
	}

	ref, err := repo.Head()
	if err != nil {
		return nil, fmt.Errorf("failed to get HEAD: %w", err)
	}

	cIter, err := repo.Log(&git.LogOptions{From: ref.Hash()})
	if err != nil {
		return nil, fmt.Errorf("failed to get log: %w", err)
	}

	commitRegex := regexp.MustCompile(`^(fix|feat|chore|docs|ci)(!?)`)
	semverRegex := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

	var latestTag string

	err = cIter.ForEach(func(c *object.Commit) error {
		summary := c.Message
		log.Info().Interface("stderr", os.Stderr).Interface("SHA", c.Hash.String()).Interface("SUMMARY", strings.Split(summary, "\n")[0]).Msg("current commit")

		// Check if this commit has a tag
		tags, _ := repo.Tags()
		_ = tags.ForEach(func(ref *plumbing.Reference) error {
			obj, err := repo.TagObject(ref.Hash())
			if err == nil && obj.Target == c.Hash {
				tagName := strings.TrimPrefix(ref.Name().Short(), "tags/")
				if semverRegex.MatchString(tagName) {
					latestTag = tagName
					return storer.ErrStop
				}
			}
			return nil
		})

		if latestTag != "" {
			return storer.ErrStop
		}

		matches := commitRegex.FindStringSubmatch(summary)
		if matches == nil {
			return fmt.Errorf("commit %s is not a conventional commit", c.Hash.String())
		}

		switch {
		case matches[2] == "!":
			isMajor = true
		case matches[1] == "feat":
			isMinor = true
		default:
			isPatch = true
		}

		return nil
	})
	if err != nil && err != storer.ErrStop {
		return nil, err
	}

	if latestTag == "" {
		return nil, fmt.Errorf("no semver tag found")
	}

	latestSemver, err := semver.NewVersion(latestTag)
	if err != nil {
		return nil, err
	}

	var newVersion semver.Version
	if isMajor {
		newVersion = latestSemver.IncMajor()
	} else if isMinor {
		newVersion = latestSemver.IncMinor()
	} else if isPatch {
		newVersion = latestSemver.IncPatch()
	} else {
		return nil, fmt.Errorf("no conventional commit type found")
	}

	return &newVersion, nil
}
