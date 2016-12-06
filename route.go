package main

// This file is used for testing

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/aryann/difflib"
	"golang.org/x/oauth2"

	githubApp "github.com/google/go-github/github"
)

// TODO - abstract client response to an interface
// type remoteClient interface {
// 	Do(req *http.Request, v interface{}) interface{}
// }

const NoneString = "<none>"

type branches struct {
	repo    *Repo
	auth    *Authentication
	client  *githubApp.Client
	option  string
	oldList []string
	newList []string
}

type branchCommits struct {
	data map[string]*branchCommit
}

type branchCommit struct {
	// branch    string
	oldCommit string
	newCommit string
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
		conf.save(filename)
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

	var diff LocalDiff
	// loop through repos and their branches
	for _, repo := range conf.Repos {
		branch.repo = repo

		if repo.Branches || len(repo.NamedReferences) > 0 {
			newBranches := getNewInfo(branch, "branches")
			// commitRefs - TODO load from file
			if len(repo.NamedReferences) > 0 {

				// 1. take all commits from conf.Info .Commits
				commitDiff := new(branchCommits)
				commitDiff.data = make(map[string]*branchCommit)
				b := conf.Info[repo.Repo]
				// TODO set newInformation as part of the config loader
				// TODO bug - if we are no longer tracking a branch, we need to remove it from the new list
				if b == nil {
					conf.Info[branch.repo.Repo] = newInformation()
					b = conf.Info[branch.repo.Repo]
				}
				for branch, commitID := range b.Commits {
					commitDiff.data[branch] = &branchCommit{
						oldCommit: commitID,
					}
				}

				diffWithOldCommits(newBranches, branch, commitDiff)

				for i, t := range commitDiff.data {
					// save new data from commitDiff.data
					if t.newCommit != NoneString {
						b.Commits[i] = t.newCommit
					}
					// fmt.Printf("%s,%s,%s\n", i, t.oldCommit, t.newCommit)
				}

				l := &BranchDiff{
					Repo:          repo.Repo,
					branchCommits: commitDiff,
				}
				diff.add(l)
			}

			if repo.Branches {
				branchesDiff := diffWithOldBranches(newBranches, branch, "branches", conf.Info)
				l := &LocalRef{
					Title:      "Branches",
					Repo:       repo.Repo,
					References: branchesDiff,
				}
				diff.add(l)
			}
		}

		if repo.Tags {
			newTags := getNewInfo(branch, "tags")
			tagsDiff := diffWithOldBranches(newTags, branch, "tags", conf.Info)
			l := &LocalRef{
				Title:      "Tags",
				Repo:       repo.Repo,
				References: tagsDiff,
			}
			diff.add(l)
		}
	}

	t := time.Now()
	subject := "[GitNotify] New Stuff from your Repositories - " + t.Format("02 Jan 2006")

	to := &recepient{
		Name:    conf.Auth.Name,
		Address: conf.Auth.Email,
	}

	ctx := &emailCtx{
		Subject:  subject,
		TextBody: diff.toText(),
		HTMLBody: diff.toHTML(),
	}

	sendEmail(to, ctx)
}

func getNewInfo(branch *branches, option string) []*TagInfo {
	branch.option = option
	branchesURL := fmt.Sprintf("%s%s/%s", githubAPIEndPoint, branch.repo.Repo, branch.option)
	fmt.Println(branchesURL)
	v := new([]*TagInfo)
	req, _ := http.NewRequest("GET", branchesURL, nil)
	branch.client.Do(req, v)
	return *v
}

func diffWithOldBranches(v []*TagInfo, branch *branches, option string, info map[string]*Information) []string {
	newBranches := make([]string, len(v))
	for i, a := range v {
		newBranches[i] = a.Name
	}

	branch.newList = newBranches
	t := info[branch.repo.Repo]
	if option == "tags" && t != nil {
		branch.oldList = t.Tags
	} else if option == "branches" && t != nil {
		branch.oldList = t.Branches
	}
	diff := getNewStrings(branch.oldList, branch.newList)
	if t == nil {
		info[branch.repo.Repo] = newInformation()
		t = info[branch.repo.Repo]
	}
	if option == "tags" {
		t.Tags = branch.newList
	} else if option == "branches" {
		t.Branches = branch.newList
	}

	return diff
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

// in the branches we are tracking,
// newcommit is "" // means that we are no longer tracking the branch/ref
// newcommit is <none> if branch is not found/deleted in remote
func diffWithOldCommits(v []*TagInfo, branch *branches, commitRef *branchCommits) {
	for _, a := range branch.repo.NamedReferences {
		s := string(a)
		c := commitRef.data[s]
		if c == nil {
			c = &branchCommit{}
			commitRef.data[s] = c
		}
		c.newCommit = findBranchCommit(v, s)
	}
}

func findBranchCommit(v []*TagInfo, branch string) string {
	for _, a := range v {
		if a.Name == branch {
			return a.Commit.Sha
		}
	}
	return NoneString
}

func init() {
	getData("github")
}

// 	r := &repoBranchCommit{}
// 	data, _ := ioutil.ReadFile("sample.yml")
// 	yaml.Unmarshal(data, &r.data)
// 	// r
// 	conf := new(Setting)
// 	conf.load("data/github/sairam/settings.yml")
//
// 	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: conf.Auth.Token})
// 	tc := oauth2.NewClient(oauth2.NoContext, ts)
// 	client := githubApp.NewClient(tc)
//
// 	branch := &branches{
// 		client: client,
// 		auth:   conf.Auth,
// 	}
// 	repo := conf.Repos[0]
// 	branch.repo = repo
//
// 	v := getNewInfo(branch, "branches")
// 	x := r.branches(repo.Repo)
// 	diffWithOldCommits(v, branch, x)
// 	for i, t := range x {
// 		fmt.Printf("%s,%s,%s\n", i, t.oldCommit, t.newCommit)
// 	}
//
// }
