package main

import (
	"encoding/json"
	"fmt"
	"github.com/carwale/golibraries/gologger"
	"github.com/carwale/golibraries/workerpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"io/ioutil"
	"net/http"
	"time"
)

type repoData struct {
	name string
	logger *gologger.CustomLogger
	completionChan chan bool
}

// getRepos returns an array of repository names on codacy
func getRepos(logger *gologger.CustomLogger) []string{

	// struct to receive and breakdown repository list json data
	type repoResults struct {
		Data []struct {
			Repository struct {
				Name         string `json:"name"`
			} `json:"repository"`
		}
	}

	req, err := http.NewRequest("GET", "https://app.codacy.com/api/v3/analysis/organizations/gh/carwale/repositories", nil)
	if err != nil{
		logger.LogError("unable to get list of repos", err)
		return nil
	}
	req.Header = map[string][]string{
		"Accept": {"application/json"},
		"api-token": {"9BdbyQFW4T4MCftcWvWw"},
	} // provides auth to http request

	client := &http.Client{Timeout: 10 * time.Second} // http client times out to prevent getting stuck while making request
	resp, err := client.Do(req)
	if err != nil{
		logger.LogError("timed out while getting list of repos", err)
		return nil
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil{
		logger.LogError("unable to unmarshal body response while getting repos", err)
		return nil
	}

	res := &repoResults{}
	err = json.Unmarshal(body, res)	// translates json response body into struct
	if err != nil{
		logger.LogError("unable to unmarshal json issues response while getting repos", err)
		return nil
	}

	var repoList []string

	// makes slice of repository names to return
	for _ , v := range res.Data {
		repoList = append(repoList, v.Repository.Name)
	}

	return repoList
}

// getIssues returns a map with issues of the respective repo
func getIssues(repoName string, logger *gologger.CustomLogger, completionChan chan bool) map[string]int {

	// struct to receive and breakdown repository issue json data
	type Results struct {
		Data []struct {
			Category struct {
				Name         string `json:"categoryType"`
			} `json:"category"`
			TotalResults int     `json:"totalResults"`
		} `json:"data"`
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("https://app.codacy.com/api/v3/analysis/organizations/gh/carwale/repositories/%s/category-overviews", repoName), nil)
	if err != nil{
		logger.LogError(fmt.Sprintf("failed to get issues for repo %s", repoName), err)
		completionChan <- false
		return nil
	}
	req.Header = map[string][]string{
		"Accept": {"application/json"},
		"api-token": {"9BdbyQFW4T4MCftcWvWw"},
	}	// provides auth to http request

	client := &http.Client{Timeout: 10 * time.Second}	// http client times out to prevent getting stuck while making request
	resp, err := client.Do(req)
	if err != nil{
		logger.LogError(fmt.Sprintf("timed out while getting issues for repo %s", repoName), err)
		completionChan <- false
		return nil
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil{
		logger.LogError(fmt.Sprintf("unable to unmarshal body response for repo %s", repoName), err)
		completionChan <- false
		return nil
	}

	res := &Results{}
	err = json.Unmarshal(body, res)	// translates json response body into struct
	if err != nil{
		logger.LogError(fmt.Sprintf("unable to unmarshal json issues response for repo %s", repoName), err)
		completionChan <- false
		return nil
	}

	issuesMap := make(map[string]int)

	// makes issuesMap to return
	for _ , v := range res.Data {
		issuesMap[v.Category.Name] = v.TotalResults
	}

	return issuesMap
}

// pushIssues pushes issue data to prometheus pushgateway
func pushIssues(repoName string, issuesMap map[string]int, logger *gologger.CustomLogger, completionChan chan bool) {

	// creates a gauge to store issue metrics
	codacyIssuesMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "codacy_issues_metric",
		Help: "Number of issues in Codacy code",
		})

	// pushes each metric in issueList
	for i , _ := range issuesMap{

		codacyIssuesMetric.Set(float64(issuesMap[i]))
		if err := push.New("http://localhost:9091", "codacy_issues_metric").
			Collector(codacyIssuesMetric).
			Grouping("Categories", i).
			Grouping("Repository", repoName).
			Push(); err != nil {
			logger.LogError(fmt.Sprintf("Could not push %s, %s to Pushgateway:", i, repoName), err)
		}

	}
	completionChan <- true
}

// Process implements IJob by combining getIssues and pushIssues
func (r *repoData) Process() error{
	issuesMap := getIssues(r.name, r.logger, r.completionChan)

	if issuesMap != nil{
		pushIssues(r.name, issuesMap, r.logger, r.completionChan)
	}

	return nil
}

func main() {

	dispatcher := workerpool.NewDispatcher("CodacyTool")

	logger := gologger.NewLogger(gologger.DisableGraylog(true), gologger.ConsolePrintEnabled(true))

	repoList := getRepos(logger)

	if repoList != nil {

		completionChan := make(chan bool, len(repoList)) // Chan used to block till all jobs are complete

		go func() {

			for _, repoName := range repoList {	// creates job for each repository in repoList

				job := &repoData{name: repoName, logger: logger, completionChan: completionChan}
				dispatcher.JobQueue <- job

			}
		}()

		for {
			if len(completionChan) == cap(completionChan){
				logger.LogErrorWithoutError("Completed All Repos")
				return
			}
		}

	}
}

