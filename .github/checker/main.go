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
	GiteaRepository = "https://github.com/go-gitea/gitea.git"
	repositoryPath  = filepath.Join(os.TempDir(), "gitea")

	dockerClient *client.Client
)

func PullAndReturnVersion(tag string) (string, error) {
	tag = fmt.Sprintf("sirherobrine23.com.br/gitea/gitea:%s", strings.ToLower(tag))
	ctx := context.Background()

	fmt.Printf("Downloading %s ...\n", tag)
	wait, err := dockerClient.ImagePull(ctx, tag, image.PullOptions{})
	if err != nil {
		if client.IsErrNotFound(err) || err.Error() == "Error response from daemon: manifest unknown" {
			return "", nil
		}
		return "", fmt.Errorf("cannot pull gitea image: %s", err.Error())
	}
	_, _ = io.Copy(io.Discard, wait) // Discart and ignore

	fmt.Println("Geting image information ...")
	imageInspect, _, err := dockerClient.ImageInspectWithRaw(ctx, tag)
	if err != nil {
		return "", fmt.Errorf("cannot get image info: %s", err.Error())
	}

	// Check if label exists and get hash registred
	hash := ""
	if imageInspect.Config.Labels != nil {
		hash = imageInspect.Config.Labels["br.com.sirherobrine23.gitea.hash"]
	}

	return hash, nil
}

func main() {
	fmt.Println("Creating docker client ...")
	err := error(nil)
	if dockerClient, err = client.NewClientWithOpts(client.FromEnv); err != nil {
		fmt.Printf("Cannot connect to docker client: %s\n", err.Error())
		os.Exit(1)
		return
	}

	fmt.Println("Cloning gitea repository")
	repo, err := git.PlainClone(repositoryPath, false, &git.CloneOptions{URL: GiteaRepository})
	if err != nil {
		if err != git.ErrRepositoryAlreadyExists {
			fmt.Printf("Cannot clone gitea: %s\n", err.Error())
			os.Exit(1)
			return
		}
		repo, _ = git.PlainOpen(repositoryPath)
		_ = repo.Fetch(&git.FetchOptions{Prune: true, Tags: git.AllTags})
	}

	tags, err := repo.Tags()
	if err != nil {
		fmt.Printf("Cannot get tags: %s\n", err.Error())
		os.Exit(1)
		return
	}
	defer tags.Close()

	tag := (*object.Tag)(nil)
	err = tags.ForEach(func(tag_refence *plumbing.Reference) error {
		if tag_refence != nil {
			ntag, err := repo.TagObject(tag_refence.Hash())
			switch err {
			default:
				return err
			case plumbing.ErrObjectNotFound:
				return nil
			case nil:
				if tag != nil {
					switch ntag.Tagger.When.Compare(tag.Tagger.When) {
					case -1:
						return nil
					}
				}
				tag = ntag
			}
		}
		return nil
	})
	if err != nil {
		fmt.Printf("Cannot get latest release: %s\n", err.Error())
		os.Exit(1)
		return
	}

	hash, err := PullAndReturnVersion(tag.Name[1:])
	if err != nil {
		fmt.Printf("Error on pull tag version: %s\n", err.Error())
		os.Exit(1)
		return
	} else if hash == "" {
		fmt.Printf("New docker release image build: %q\n", tag.Name[1:])
		gha.SetOutput("git_hash", tag.Hash.String())
		gha.SetOutput("docker_tag", tag.Name[1:])
		gha.SetOutput("skip", "0")
		return
	}

	mainBranch, err := repo.Reference(plumbing.Main, true)
	if err != nil {
		fmt.Printf("Cannot get main branch: %s\n", err.Error())
		os.Exit(1)
		return
	}

	gitHash := mainBranch.Hash().String()
	if gitHash != hash {
		fmt.Printf("New docker image build, %q => %q\n", hash, gitHash)
		gha.SetOutput("git_hash", gitHash)
		gha.SetOutput("skip", "0")
	} else {
		fmt.Println("Skiping docker image build, ared published")
		gha.SetOutput("skip", "1")
	}
	gha.SetOutput("docker_tag", "latest")
}
