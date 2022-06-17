# Code Monitoring Tool

This tool was made with the purpose of providing an easy way of viewing Codacy 
metrics in Grafana since Codacy doesn't have the best visual analytics.

The following steps are followed by the tool:
* GET requests are sent to the organization repository to collect the list of repositories.
* Data on these repositories are then concurrently collected using a workerpool.
* These metrics are then uploaded to Prometheus Gauge metrics using the pushgateway. 
* The issues covered include syntax, style and all the other issues supported by codacy.

**NOTE**: You need prometheus and prometheus pushgateway installed on your machine for this to work.

A [Codacy API Key][key] is required to run this tool.

[key]: https://docs.codacy.com/codacy-api/api-tokens/

```
$ git clone https://github.com/sumerpunjabi/CodeMonitoringTool
$ go get
$ go build
```
