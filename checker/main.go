package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"golang.org/x/mod/semver"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	gha "github.com/sethvargo/go-githubactions"
)

var (
	Repository   *git.Repository
	OriginRemote *git.Remote
)

type Output struct {
	DockerTag    string `json:"DOCKER_TAG"`
	GiteaTag     string `json:"GITEA_TAG"`
	GiteaVersion string `json:"GITEA_VERSION"`
}

func init() {
	var err error

	// Setup envs
	repositoryPath := filepath.Join(os.TempDir(), "gitea")
	GiteaRepositoryURL := os.Getenv("GITEA_REPO")
	if GiteaRepositoryURL == "" {
		GiteaRepositoryURL = "https://github.com/go-gitea/gitea.git"
	}

	cloneConfig := git.CloneOptions{
		URL:      GiteaRepositoryURL,
		Tags:     git.AllTags,
		Progress: os.Stdout,
	}

	fmt.Println("Cloning gitea repository ...")
	if Repository, err = git.PlainClone(repositoryPath, false, &cloneConfig); err != nil {
		if err != git.ErrRepositoryAlreadyExists {
			fmt.Printf("Cannot clone gitea: %s\n", err.Error())
			os.Exit(1)
			return
		}

		fmt.Println("Repository are exist, opening ...")
		if Repository, err = git.PlainOpen(repositoryPath); err != nil {
			fmt.Printf("Cannot open gitea repository: %s\n", err.Error())
			os.Exit(1)
			return
		}
	}

	rms, err := Repository.Remotes()
	if err != nil {
		fmt.Printf("Cannot get local repository remotes: %s\n", err.Error())
		os.Exit(1)
		return
	}

	for _, r := range rms {
		if slices.Contains(r.Config().URLs, GiteaRepositoryURL) {
			OriginRemote = r
			break
		}
	}

	if OriginRemote == nil {
		newRepo := strings.ToLower(rand.Text())
		fmt.Printf("Add new remote %q: %s\n", newRepo, GiteaRepositoryURL)
		OriginRemote, err = Repository.CreateRemote(&config.RemoteConfig{
			Name: newRepo,
			URLs: []string{GiteaRepositoryURL},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Cannot add repository reference: %s", err)
			os.Exit(1)
			return
		}
	}

	fmt.Printf("Gettings changes from %q remote repository ...\n", OriginRemote.Config().Name)
	if err = OriginRemote.Fetch(&git.FetchOptions{Prune: true, Tags: git.AllTags, Progress: os.Stdout}); err != nil {
		fmt.Printf("Cannot fetch new changes: %s\n", err.Error())
		os.Exit(1)
		return
	}

	fmt.Println("Respository cloned!")
}

func GetVersionFromRegistry(tag string) (string, error) {
	imageRef := fmt.Sprintf(
		"ghcr.io/sirherobrine23/gitea:%s",
		strings.ToLower(tag),
	)

	ctx := context.Background()

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("invalid image reference %s: %w", imageRef, err)
	}

	fmt.Printf("Getting image info from registry: %s ...\n", imageRef)
	img, err := remote.Image(
		ref,
		remote.WithContext(ctx),
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithPlatform(v1.Platform{
			OS:           "linux",
			Architecture: "amd64",
		}),
	)
	if err != nil {
		var terr *transport.Error
		if errors.As(err, &terr) && terr.StatusCode == http.StatusNotFound {
			return "", nil
		}

		return "", fmt.Errorf("cannot get image %s from registry: %w", imageRef, err)
	}

	config, err := img.ConfigFile()
	if err != nil {
		return "", fmt.Errorf("cannot get config for %s: %w", imageRef, err)
	}

	if config.Config.Labels == nil {
		return "", nil
	}

	return config.Config.Labels["br.com.sirherobrine23.gitea.hash"], nil
}

func LatestTag(devel bool) (*object.Commit, string, error) {
	tags, err := Repository.Tags()
	if err != nil {
		return nil, "", err
	}
	defer tags.Close()

	var (
		commit, currentCommit *object.Commit
		tagReference          string
	)

	for {
		tager, err := tags.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, "", err
		}

		if currentTag, err := Repository.TagObject(tager.Hash()); err != nil {
			if err != plumbing.ErrObjectNotFound {
				return nil, "", err
			} else if !devel {
				continue
			} else if currentCommit, err = Repository.CommitObject(tager.Hash()); err != nil {
				return nil, "", err
			}
		} else if currentCommit, err = currentTag.Commit(); err != nil {
			return nil, "", err
		}

		if commit == nil || currentCommit.Committer.When.Compare(commit.Committer.When) >= 0 {
			tagReference = tager.Name().Short()
			commit = currentCommit
		}
	}

	return commit, tagReference, nil
}

func getDescribeLike(repo *git.Repository, head *plumbing.Reference) string {
	tags, _ := repo.Tags()
	shortHash := head.Hash().String()[:7]
	var currentTag string

	_ = tags.ForEach(func(t *plumbing.Reference) error {
		if currentTag == "" {
			currentTag = t.Name().Short()
			return nil
		}
		if semver.Compare(t.Name().Short(), currentTag) >= 0 {
			currentTag = t.Name().Short()
		}
		return nil
	})

	if currentTag == "" {
		return "main-" + shortHash
	}

	return strings.TrimLeft(currentTag+"-"+shortHash, "v")
}

func main() {
	Releases := []*Output{}

	fmt.Println("Geting latest tag release")
	_, tagRelease, err := LatestTag(false)
	if err != nil {
		fmt.Printf("Cannot get latest release: %s\n", err.Error())
		os.Exit(1)
		return
	}

	refRemoteMain := plumbing.NewRemoteReferenceName(OriginRemote.Config().Name, "main")
	mainBranch, err := Repository.Reference(refRemoteMain, true)
	if err != nil {
		fmt.Printf("Cannot get main branch: %s\n", err.Error())
		os.Exit(1)
		return
	}

	if hash, err := GetVersionFromRegistry(tagRelease); err != nil || hash == "" {
		fmt.Printf("New tag release: %q\n", tagRelease)
		Releases = append(Releases, &Output{
			DockerTag:    tagRelease,
			GiteaTag:     tagRelease,
			GiteaVersion: tagRelease,
		})
	}

	gitea_version := getDescribeLike(Repository, mainBranch)
	if gitea_version == "" {
		gitea_version = "main-nightly"
	}

	if hash, err := GetVersionFromRegistry("latest"); !(err != nil || hash == gitea_version || strings.HasPrefix(mainBranch.Hash().String(), hash)) {
		fmt.Printf("New nightly docker build, %q => %q\n", hash, gitea_version)
		Releases = append(Releases, &Output{
			DockerTag:    "latest",
			GiteaTag:     "main",
			GiteaVersion: gitea_version,
		})
	}

	if jsValue, err := json.Marshal(Releases); err == nil {
		if os.Getenv("GITHUB_ACTIONS") != "" {
			gha.SetOutput("BUILDS", string(jsValue))
			gha.SetOutput("BUILDS_COUNT", strconv.Itoa(len(Releases)))
		}
	}
	if jsValue, err := json.MarshalIndent(Releases, "", "  "); err == nil {
		println(string(jsValue))
	}
}
