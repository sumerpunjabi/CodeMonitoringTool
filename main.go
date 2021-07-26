package main

import (
	"encoding/json"
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
	"io/ioutil"
	"log"
	"net/http"
	"sync"
	"time"
)


// getRepos returns an array of repository names on codacy
func getRepos() []string{

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
	}

	client := &http.Client{Timeout: 10 * time.Second}
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
	err = json.Unmarshal(body, res)

	var temp []string

	for _ , v := range res.Data {
		temp = append(temp, v.Repository.Name)
	}

	return temp
}

// issues returns a buffered integer channel with issues of the respective repo
func issues(name string, chan1 chan int) {

	type Results struct {
		Data []struct {
			Category struct {
				Name         string `json:"name"`
			} `json:"category"`
			TotalResults int     `json:"totalResults"`
		} `json:"data"`
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("https://app.codacy.com/api/v3/analysis/organizations/gh/carwale/repositories/%s/category-overviews", name), nil)
	if err != nil{
		log.Fatalln(err)
	}
	req.Header = map[string][]string{
		"Accept": {"application/json"},
		"api-token": {"9BdbyQFW4T4MCftcWvWw"},
	}

	client := &http.Client{Timeout: 10 * time.Second}
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
	err = json.Unmarshal(body, res)

	for _ , v := range res.Data {
		chan1 <- v.TotalResults
	}

}

// pushIssues pushes issue data to prometheus pushgateway
func pushIssues(name string, chan1 chan int) {

	issueList := []string{"codeStyle", "security", "errorProne", "performance", "compatibility", "complexity", "documentation", "unusedCode"}

	// creates a gauge to store issue metrics
	codacyIssuesMetric := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "codacy_issues_metric",
		Help: "Number of issues in Codacy code",
		})

	// pushes each metric in issueList
	for _ , v := range issueList{

		codacyIssuesMetric.Set(float64(<-chan1))
		if err := push.New("http://localhost:9091", name).
			Collector(codacyIssuesMetric).
			Grouping("Categories", v).
			Push(); err != nil {
			fmt.Println("Could not push completion time to Pushgateway:", err)
		}

	}

}

// Worker defines the job to be by every time a worker is added to the pool
func Worker(name string, wg *sync.WaitGroup) {

	defer wg.Done()	// removes worker from pool after completion of job

	chan1 := make(chan int , 8)

	issues(name, chan1)

	pushIssues(name, chan1)
}


func main() {

	var wg sync.WaitGroup

	repos := getRepos()

	// adds one worker to the pool per repository
	for _ , v := range repos{
		wg.Add(1)
		go Worker(v, &wg)
	}

	wg.Wait()	// waits for all workers to finish

}

/*
Notes:

Channels and strings are passed by value in parameters because no change is expected to their values
and passing by value is a lot less expensive to the memory

Used a Buffered channel instead of passing through a normal channel because we
don't have to create a slice or struct to pass the values between functions and maps
are unreliable when used with goroutines

*/
