package main

import (
	"encoding/json"
	"fmt"
	"github.com/gammazero/workerpool"
	"github.com/joho/godotenv"
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/gologger/levels"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

type repoData struct {
	key            string
	org            string
	repoName       string
	completionChan chan bool
}

// getRepos returns an array of repository names on codacy
func getRepos(key string, org string) []string {

	// struct to receive and breakdown repository list json data
	type repoResults struct {
		Data []struct {
			Repository struct {
				Name string `json:"name"`
			} `json:"repository"`
		}
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("https://app.codacy.com/api/v3/analysis/organizations/gh/%s/repositories", org), nil)
	if err != nil {
		gologger.Warning().Str("state", "errored").Str("status", "404").
			Msg("Unable to get repo list")
		return nil
	}
	req.Header = map[string][]string{
		"Accept":    {"application/json"},
		"api-token": {key},
	} // provides auth to http request

	client := &http.Client{Timeout: 10 * time.Second} // http client times out to prevent getting stuck while making request
	resp, err := client.Do(req)
	if err != nil {
		gologger.Warning().Str("state", "errored").Str("status", "404").
			Msg("timed out while getting list of repo")
		return nil
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		gologger.Warning().Str("state", "errored").Str("status", "404").
			Msg("unable to unmarshal body response while getting repos")
		return nil
	}

	res := &repoResults{}
	err = json.Unmarshal(body, res) // translates json response body into struct
	if err != nil {
		gologger.Warning().Str("state", "errored").Str("status", "404").
			Msg("unable to unmarshal json issues response while getting repos")
		return nil
	}

	var repoList []string

	// makes slice of repository names to return
	for _, v := range res.Data {
		repoList = append(repoList, v.Repository.Name)
	}

	return repoList
}

// getIssues returns a map with issues of the respective repo
func (r *repoData) getIssues() map[string]int {

	// struct to receive and breakdown repository issue json data
	type Results struct {
		Data []struct {
			Category struct {
				Name string `json:"categoryType"`
			} `json:"category"`
			TotalResults int `json:"totalResults"`
		} `json:"data"`
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("https://app.codacy.com/api/v3/analysis/organizations/gh/%s/repositories/%s/category-overviews", r.org, r.repoName), nil)
	if err != nil {
		gologger.Warning().Str("state", "errored").Str("status", "404").
			Msg(fmt.Sprintf("failed to get issues for repo %s", r.repoName))
		r.completionChan <- false
		return nil
	}
	req.Header = map[string][]string{
		"Accept":    {"application/json"},
		"api-token": {r.key},
	} // provides auth to http request

	client := &http.Client{Timeout: 10 * time.Second} // http client times out to prevent getting stuck while making request
	resp, err := client.Do(req)
	if err != nil {
		gologger.Warning().Str("state", "errored").Str("status", "404").
			Msg(fmt.Sprintf("timed out while getting issues for repo %s", r.repoName))
		r.completionChan <- false
		return nil
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		gologger.Warning().Str("state", "errored").Str("status", "404").
			Msg(fmt.Sprintf("unable to unmarshal body response for repo %s", r.repoName))
		r.completionChan <- false
		return nil
	}

	res := &Results{}
	err = json.Unmarshal(body, res) // translates json response body into struct
	if err != nil {
		gologger.Warning().Str("state", "errored").Str("status", "404").
			Msg(fmt.Sprintf("unable to unmarshal json issues response for repo %s", r.repoName))
		r.completionChan <- false
		return nil
	}

	issuesMap := make(map[string]int)

	// makes issuesMap to return
	for _, v := range res.Data {
		issuesMap[v.Category.Name] = v.TotalResults
	}

	return issuesMap
}

// pushIssues pushes issue data to prometheus pushgateway
func (r *repoData) pushIssues(issuesMap map[string]int) {

	// creates a gauge to store issue metrics
	codacyIssuesMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "codacy_issues_metric",
		Help: "Number of issues in Codacy code",
	})

	// pushes each metric in issueList
	for i := range issuesMap {

		codacyIssuesMetric.Set(float64(issuesMap[i]))
		if err := push.New("http://localhost:9091", "codacy_issues_metric").
			Collector(codacyIssuesMetric).
			Grouping("Categories", i).
			Grouping("Repository", r.repoName).
			Push(); err != nil {
			gologger.Warning().Str("state", "errored").Str("status", "404").
				Msg(fmt.Sprintf("Could not push %s, %s to Pushgateway:", i, r.repoName))
		}
	}
	r.completionChan <- true
}

// Process implements IJob by combining getIssues and pushIssues
func (r *repoData) Process() error {
	issuesMap := r.getIssues()

	if issuesMap != nil {
		r.pushIssues(issuesMap)
	}

	return nil
}

func main() {

	err := godotenv.Load("local.env")
	key := os.Getenv("KEY")
	org := os.Getenv("ORG")

	if err != nil {
		gologger.Fatal().Msg("Failed to retrieve api key")
		return
	}

	gologger.DefaultLogger.SetMaxLevel(levels.LevelDebug)

	repoList := getRepos(key, org)

	if repoList != nil {

		dispatcher := workerpool.New(len(repoList) / 5)

		completionChan := make(chan bool, len(repoList)) // Chan used to block till all jobs are complete

		go func() {

			for _, repoName := range repoList { // creates job for each repository in repoList

				job := &repoData{key: key, org: org, repoName: repoName, completionChan: completionChan}
				dispatcher.Submit(func() {
					issues := job.getIssues()
					job.pushIssues(issues)
				})

			}
		}()

		for {
			if len(completionChan) == cap(completionChan) {
				gologger.Print().Msgf("Completed All Repos\n")
				return
			}
		}

	}
}
