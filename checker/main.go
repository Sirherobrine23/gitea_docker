package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"golang.org/x/mod/semver"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"

	gha "github.com/sethvargo/go-githubactions"
)

var (
	Repository *git.Repository

	dockerClient *client.Client

	Releases map[string]*Output = map[string]*Output{}
)

type Output struct {
	DockerTag    string `json:"DOCKER_TAG"`
	GiteaTag     string `json:"GITEA_TAG"`
	GiteaVersion string `json:"GITEA_VERSION"`
}

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
	tag = fmt.Sprintf("ghcr.io/sirherobrine23/gitea:%s", strings.ToLower(tag))
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

func getDescribeLike(repo *git.Repository, head *plumbing.Reference) string {
	tags, _ := repo.Tags()
	var currentTag string

	_ = tags.ForEach(func(t *plumbing.Reference) error {
		if currentTag == "" {
			currentTag = t.Name().Short()
			return nil
		}
		if semver.Compare(t.Name().Short(), currentTag) >= 0 {
			fmt.Println(currentTag, t.Name().Short())
			currentTag = t.Name().Short()
		}
		return nil
	})

	if currentTag != "" {
		return strings.TrimLeft(currentTag+"-"+head.Hash().String()[:7], "v")
	}

	return "main-" + head.Hash().String()[:7]
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

	if hash, err := PullAndReturnVersion(tagRelease); err != nil || hash == "" {
		fmt.Printf("New tag release: %q\n", tagRelease)
		Releases["release"] = &Output{
			DockerTag:    tagRelease,
			GiteaTag:     tagRelease,
			GiteaVersion: tagRelease,
		}
	}

	gitea_version := getDescribeLike(Repository, mainBranch)
	if gitea_version == "" {
		gitea_version = "main-nightly"
	}

	if hash, err := PullAndReturnVersion("latest"); !(err != nil || hash == gitea_version || strings.HasPrefix(mainBranch.Hash().String(), hash)) {
		fmt.Printf("New nightly docker build, %q => %q\n", hash, gitea_version)
		Releases["nightly"] = &Output{
			DockerTag:    "latest",
			GiteaTag:     "main",
			GiteaVersion: gitea_version,
		}
	}

	for key, value := range Releases {
		data, err := json.Marshal(value)
		if err != nil {
			fmt.Printf("Error on set builds: %s\n", err.Error())
			os.Exit(1)
			return
		}
		gha.SetOutput(key, string(data))
	}
}
