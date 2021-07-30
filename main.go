package main

import (
	"encoding/json"
	"fmt"
	"github.com/gammazero/workerpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)


// getRepos returns an array of repository names on codacy
func getRepos() []string{

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
		log.Fatalln(err)
	}
	req.Header = map[string][]string{
		"Accept": {"application/json"},
		"api-token": {"9BdbyQFW4T4MCftcWvWw"},
	} // provides auth to http request

	client := &http.Client{Timeout: 10 * time.Second} // http client times out to prevent getting stuck while making request
	resp, err := client.Do(req)
	if err != nil{
		log.Fatalln(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil{
		log.Fatalln(err)
	}

	res := &repoResults{}
	err = json.Unmarshal(body, res)	// translates json response body into struct
	if err != nil{
		log.Fatalln(err)
	}

	var repoList []string

	// makes slice of repository names to return
	for _ , v := range res.Data {
		repoList = append(repoList, v.Repository.Name)
	}

	return repoList
}

// issues returns a buffered integer channel with issues of the respective repo
func getIssues(repoName string) map[string]int {

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
		log.Fatalln(err)
	}
	req.Header = map[string][]string{
		"Accept": {"application/json"},
		"api-token": {"9BdbyQFW4T4MCftcWvWw"},
	}	// provides auth to http request

	client := &http.Client{Timeout: 10 * time.Second}	// http client times out to prevent getting stuck while making request
	resp, err := client.Do(req)
	if err != nil{
		log.Fatalln(err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil{
		log.Fatalln(err)
	}

	res := &Results{}
	err = json.Unmarshal(body, res)	// translates json response body into struct
	if err != nil{
		log.Fatalln(err)
	}

	issuesMap := make(map[string]int)

	// returns issue values to buffered channel for further use
	for _ , v := range res.Data {
		issuesMap[v.Category.Name] = v.TotalResults
	}

	return issuesMap
}

// pushIssues pushes issue data to prometheus pushgateway
func pushIssues(repoName string, issuesMap map[string]int) {

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
			fmt.Printf("Could not push %s, %s to Pushgateway:", i, repoName)
			fmt.Println(err)
		}

	}

}


func main() {

	repos := getRepos()

	wg := workerpool.New(10)

	// adds one worker to the pool per repository
	for _ , repoName := range repos{

		wg.Submit(func() {
			issuesMap := getIssues(repoName)

			pushIssues(repoName, issuesMap)
		})
	}

	wg.StopWait()	// waits for all workers to finish

}
