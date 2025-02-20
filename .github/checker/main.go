package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"sirherobrine23.com.br/go-bds/go-bds/request/v2"

	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	gha "github.com/sethvargo/go-githubactions"
)

func main() {
	fmt.Println("Creating docker client ...")
	client, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		fmt.Printf("Cannot connect to docker client: %s\n", err.Error())
		os.Exit(1)
		return
	}
	ctx := context.Background()

	fmt.Println("Downloading latest gitea image ...")
	wait, err := client.ImagePull(ctx, "sirherobrine23.com.br/gitea/gitea:latest", image.PullOptions{})
	if err != nil {
		fmt.Printf("Cannot pull latest gitea image: %s\n", err.Error())
		os.Exit(1)
		return
	}
	_, _ = io.Copy(io.Discard, wait) // Discart and ignore

	fmt.Println("Geting image information ...")
	imageInspect, _, err := client.ImageInspectWithRaw(ctx, "sirherobrine23.com.br/gitea/gitea:latest")
	if err != nil {
		fmt.Printf("Cannot get image info: %s\n", err.Error())
		os.Exit(1)
		return
	}

	// Check if label exists and get hash registred
	hash := ""
	if imageInspect.Config.Labels != nil {
		hash = imageInspect.Config.Labels["br.com.sirherobrine23.gitea.hash"]
	}

	type base struct {
		Commit struct {
			SHA string `json:"sha"`
		} `json:"base_commit"`
	}

	gitHash, _, err := request.JSON[base]("https://api.github.com/repos/go-gitea/gitea/compare/main...HEAD", nil)
	if err != nil {
		fmt.Printf("Cannot get gitea git hash: %s\n", err.Error())
		os.Exit(1)
		return
	}

	// Ignore if ared published
	if gitHash.Commit.SHA != hash {
		fmt.Printf("New docker image build, %q => %q\n", hash, gitHash.Commit.SHA)
		gha.SetOutput("git_hash", gitHash.Commit.SHA)
		gha.SetOutput("skip", "0")
	} else {
		fmt.Println("Skiping docker image build, ared published")
		gha.SetOutput("skip", "1")
	}
}
