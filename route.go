package main

// This file is used for testing

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/aryann/difflib"
	"golang.org/x/oauth2"

	githubApp "github.com/google/go-github/github"
)

type branches struct {
	repo    *Repo
	auth    *Authentication
	client  *githubApp.Client
	option  string
	oldList []string
	newList []string
}

func fetchFiles(provider string) []string {

	dir := fmt.Sprintf("%s/%s", dataDir, provider)
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		log.Println(err)
		return []string{}
	}
	files := make([]string, len(fis))
	for i, fi := range fis {
		if fi.IsDir() {
			files[i] = dir + "/" + fi.Name() + "/" + settingsFile
		}
	}
	return files
}

func getData(provider string) {
	files := fetchFiles("github")
	for i, filename := range files {
		if filename == "" {
			continue
		}
		conf := new(Setting)
		log.Printf("Processing file %d - %s\n", i, filename)
		conf.load(filename)
		process(conf)
	}
}

func process(conf *Setting) {
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: conf.Auth.Token})
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	client := githubApp.NewClient(tc)

	branch := &branches{
		client: client,
		auth:   conf.Auth,
	}

	diff := ""
	// loop through repos and their branches
	for _, repo := range conf.Repos {
		branch.repo = repo

		if repo.Branches {
			branchesDiff := updateNewBranches(branch, "branches")
			if len(branchesDiff) > 0 {
				diff += "New branches for " + repo.Repo + "\n" + strings.Join(branchesDiff, "\n") + "\n"
			} else {
				diff += "No New branches created today for " + repo.Repo + "\n"
			}
		}

		if repo.Tags {
			tagsDiff := updateNewBranches(branch, "tags")
			if len(tagsDiff) > 0 {
				diff += "New tags for " + repo.Repo + "\n" + strings.Join(tagsDiff, "\n") + "\n"
			} else {
				diff += "No New tags created today for " + repo.Repo + "\n"
			}
		}
	}

	to := &recepient{
		Name:    "Sairam",
		Address: "sairam.kunala@gmail.com",
	}

	t := time.Now()
	ctx := &emailCtx{
		Subject: "[GitNotify] Diff for Your Repositories - " + t.Format("02 Jan 2006"),
		Body:    diff,
	}

	sendEmail(to, ctx)
}

func updateNewBranches(branch *branches, option string) []string {
	branch.option = option
	branchesURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", branch.repo.Repo, branch.option)
	fmt.Println(branchesURL)
	v := new([]*BranchInfo)
	req, _ := http.NewRequest("GET", branchesURL, nil)
	branch.client.Do(req, v)
	newBranches := make([]string, len(*v))
	for i, a := range *v {
		newBranches[i] = a.Name
	}

	branch.newList = newBranches
	branch.load()
	diff := branch.diff()
	branch.save()
	return diff
}

// check data difference with previously saved one
func (b *branches) diff() []string {
	return getNewStrings(b.oldList, b.newList)
}

func (b *branches) fileName() string {
	repo := b.repo
	fileName := strings.Replace(repo.Repo, "/", "__", 1)
	dir := fmt.Sprintf("data/%s/%s/repo", b.auth.Provider, b.auth.UserName)
	if _, err := os.Stat(dir); err != nil {
		os.Mkdir(dir, 0700)
	}
	return fmt.Sprintf("%s/%s-%s.yml", dir, fileName, b.option)
}

// load copies data into oldList
func (b *branches) load() error {
	data, err := ioutil.ReadFile(b.fileName())
	if os.IsNotExist(err) {
		return err
	}

	err = yaml.Unmarshal(data, &b.oldList)
	if err != nil {
		return err
	}

	return nil
}

// save copies data from newList into file
func (b *branches) save() error {
	out, err := yaml.Marshal(b.newList)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(b.fileName(), out, 0600)
}

func getNewStrings(old, new []string) []string {
	var strs []string
	for _, s := range difflib.Diff(old, new) {
		if s.Delta == difflib.RightOnly {
			strs = append(strs, s.Payload)
		}
	}
	return strs
}

func init() {
	getData("github")
}
