# Code Monitoring Tool

This tool uses codacy metrics to collect the list of repositories.
Data on these repositories are then concurrently collected using codacy metrics
and analysis. They are then uploaded to Prometheus Gauge metrics. 
These issues include syntax, style and all the other issues supported by codacy.
These metrics can be linked to other platforms such as Grafana to make them pretty to view.

A [Codacy API Key][key] is required to run this tool.

[key]: https://docs.codacy.com/codacy-api/api-tokens/

```
$ git clone https://github.com/sumerpunjabi/CodeMonitoringTool
$ go get
$ go build
```
