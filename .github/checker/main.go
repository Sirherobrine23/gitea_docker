package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	gha "github.com/sethvargo/go-githubactions"
)

var (
	Repository *git.Repository

	dockerClient *client.Client
)

func init() {
	fmt.Println("Creating docker client ...")
	err := error(nil)
	if dockerClient, err = client.NewClientWithOpts(client.FromEnv); err != nil {
		fmt.Printf("Cannot connect to docker client: %s\n", err.Error())
		os.Exit(1)
		return
	}

	// Setup envs
	repositoryPath := filepath.Join(os.TempDir(), "gitea")
	GiteaRepository := os.Getenv("GITEA_REPO")
	if GiteaRepository == "" {
		GiteaRepository = "https://github.com/go-gitea/gitea.git"
	}

	fmt.Println("Cloning gitea repository ...")
	if Repository, err = git.PlainClone(repositoryPath, false, &git.CloneOptions{URL: GiteaRepository, Tags: git.AllTags}); err != nil {
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

		fmt.Println("Updating repository ...")
		if err = Repository.Fetch(&git.FetchOptions{Prune: true, Tags: git.AllTags}); err != nil {
			fmt.Printf("Cannot fetch new changes: %s\n", err.Error())
			os.Exit(1)
			return
		}
	}
	fmt.Println("Respository cloned!")
}

func PullAndReturnVersion(tag string) (string, error) {
	tag = fmt.Sprintf("sirherobrine23.com.br/gitea/gitea:%s", strings.ToLower(tag))
	ctx := context.Background()

	fmt.Printf("Downloading %s ...\n", tag)
	if wait, err := dockerClient.ImagePull(ctx, tag, image.PullOptions{}); err == nil {
		_, _ = io.Copy(io.Discard, wait) // Pull and ignore daemon pulling log
	} else {
		switch errTxt := err.Error(); errTxt {
		case "Error response from daemon: manifest unknown":
			return "", nil // Return no hash avaible
		default:
			return "", fmt.Errorf("cannot pull %s image: %s", tag, errTxt)
		}
	}

	// Check if label exists and get hash registred
	fmt.Println("Geting image information ...")
	if imageInspect, _, err := dockerClient.ImageInspectWithRaw(ctx, tag); err == nil {
		if imageInspect.Config.Labels != nil {
			return imageInspect.Config.Labels["br.com.sirherobrine23.gitea.hash"], nil
		}
	}
	return "", nil
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

func setTargetInfo(skip bool, target, dockerTag, gitVersion, giteaVersion string) {
	target = strings.ToUpper(target)
	if skip {
		gha.SetOutput(fmt.Sprintf("%s_SKIP", target), "1")
		return
	}
	gha.SetOutput(fmt.Sprintf("%s_SKIP", target), "0")
	gha.SetOutput(fmt.Sprintf("%s_DOCKER_TAG", target), dockerTag)
	gha.SetOutput(fmt.Sprintf("%s_GIT_VERSION", target), gitVersion)
	gha.SetOutput(fmt.Sprintf("%s_GITEA_VERSION", target), giteaVersion)
}

func main() {
	fmt.Println("Geting latest tag release")
	_, tagRelease, err := LatestTag(false)
	if err != nil {
		fmt.Printf("Cannot get latest release: %s\n", err.Error())
		os.Exit(1)
		return
	}

	mainBranch, err := Repository.Reference(plumbing.Main, true)
	if err != nil {
		fmt.Printf("Cannot get main branch: %s\n", err.Error())
		os.Exit(1)
		return
	}

	if hash, err := PullAndReturnVersion(tagRelease); err != nil || hash != "" {
		fmt.Println("Dont have new tag release")
		setTargetInfo(true, "release", "", "", "")
	} else if hash == "" {
		fmt.Printf("New tag release: %q\n", tagRelease)
		setTargetInfo(false, "release", tagRelease, tagRelease, tagRelease)
	}

	if hash, err := PullAndReturnVersion("latest"); err != nil || hash == mainBranch.Hash().String() {
		fmt.Println("No have new 'nightly' build")
		setTargetInfo(true, "latest", "", "", "")
	} else {
		fmt.Printf("New nightly docker build, %q => %q\n", hash, mainBranch.Hash().String())
		setTargetInfo(false, "latest", "latest", "main", mainBranch.Hash().String())
	}
}
