package main

import (
	"context"
	"fmt"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	plumbingHttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/google/go-github/v56/github"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

func main() {
	ctx := context.Background()

	errorLog := log.New(os.Stderr, "", 0)

	repo, found := os.LookupEnv("REPOSITORY")
	if !found {
		errorLog.Println("repository missing")
		os.Exit(1)
	}

	owner, found := os.LookupEnv("OWNER")
	if !found {
		errorLog.Println("owner missing")
		os.Exit(1)
	}

	token, found := os.LookupEnv("GITHUB_TOKEN")
	if !found {
		errorLog.Println("github token missing")
		os.Exit(1)
	}

	client := github.NewClient(http.DefaultClient).WithAuthToken(token)

	repository, _, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		errorLog.Println(err.Error())
		os.Exit(1)
	}

	commitSha, _, err := client.Repositories.GetCommitSHA1(ctx, owner, repo,
		fmt.Sprintf("heads/%s", *repository.DefaultBranch), "")
	if err != nil {
		errorLog.Println(err.Error())
		os.Exit(1)
	}

	branchName := fmt.Sprintf("suggestions-%s", commitSha)
	pullRequests, _, err := client.PullRequests.List(ctx, owner, repo, &github.PullRequestListOptions{
		Base: *repository.DefaultBranch,
	})

	for _, pr := range pullRequests {
		if *pr.Head.Ref == branchName {
			log.Println("a pull request already exists")
			return
		}
	}

	directory, err := os.MkdirTemp(os.TempDir(), "clone-")
	if err != nil {
		errorLog.Println(err.Error())
		os.Exit(1)
	}
	defer os.RemoveAll(directory)

	clonedRepo, err := git.PlainClone(directory, false, &git.CloneOptions{
		URL: *repository.CloneURL,
		Auth: &plumbingHttp.BasicAuth{
			Username: "empty",
			Password: token,
		},
		Depth:    1,
		Progress: os.Stdout,
	})

	if err != nil {
		errorLog.Println(err.Error())
		os.Exit(1)
	}

	head, _ := clonedRepo.Head()
	ref := plumbing.NewHashReference(plumbing.ReferenceName(fmt.Sprintf("refs/heads/%s",
		branchName)), head.Hash())
	clonedRepo.Storer.SetReference(ref)
	wt, err := clonedRepo.Worktree()
	if err != nil {
		log.Println(err.Error())
		os.Exit(1)
	}

	err = wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
	})
	if err != nil {
		errorLog.Println(err.Error())
		os.Exit(1)
	}

	log.Println("running ansible lint")

	cmd := exec.Command("/home/scanner/venv/bin/ansible-lint",
		"--fix",
		"--config-file=./ansible-lint-config.yml",
		directory)
	cmd.Dir = "/home/scanner"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()

	if err != nil {
		log.Println("== error running ansible lint")
		errorLog.Println(err.Error())
		os.Exit(1)
	}

	st, err := wt.Status()
	if err != nil {
		log.Println("== error running git status")
		errorLog.Println(err.Error())
		os.Exit(1)
	}

	if !st.IsClean() {
		_, err = wt.Commit("go bot recommendations", &git.CommitOptions{
			All: true,
			Author: &object.Signature{
				Name:  "Go Bot",
				Email: "pbraun@redhat.com",
				When:  time.Now().UTC(),
			},
		})
		if err != nil {
			log.Println("== error running git commit")
			errorLog.Println(err.Error())
			os.Exit(1)
		}

		err = clonedRepo.Push(&git.PushOptions{
			Auth: &plumbingHttp.BasicAuth{
				Username: "empty",
				Password: token,
			},
		})
		if err != nil {
			log.Println("== error running git push")
			errorLog.Println(err.Error())
			os.Exit(1)
		}

		_, _, err = client.PullRequests.Create(ctx, owner, repo, &github.NewPullRequest{
			Title:               github.String("Bot Suggestions"),
			Head:                github.String(branchName),
			Base:                repository.DefaultBranch,
			Body:                github.String("Bot Suggestions"),
			MaintainerCanModify: github.Bool(true),
		})
		if err != nil {
			log.Println("== error creating pull request")
			errorLog.Println(err.Error())
			os.Exit(1)
		}
	}
}
