package web_service_overview

import (
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var versionOverviewTemplate = template.Must(template.New("versionOverview").Parse(`<!DOCTYPE html>
{{ $envs := .Environments }}
{{ $services := .WebServices }}
{{ $rows := .Rows }}
<style>
    .A {
        background-color: beige;
    }

    .B {
        background-color: azure;
    }

    .error {
        background-color: red;
    }
</style>
<html>
<head>
    <title>Results</title>
</head>
<body>
<table>
    <tr style="font-weight: bold">
        <td></td>
        {{range .Environments}}
            <td><a href="{{.BaseUrl}}">{{.Name}}</a></td> {{else}} (No environments found) {{end}}
    </tr>
    {{range $row := $rows}}
        <tr {{ if ($row.Even) }} class="A" {{else}} class="B" {{end}}>
            <td>{{$row.Name}}</td>
            {{range $cell := $row.Cells}}
                <td {{ if $cell.Content.IsError}} class="error" {{end}} title="{{$cell.Content.Title}}">{{ $cell.Content.Text}}</td>
            {{end}}
        </tr> {{end}}
</table>
</body>
</html>`))

var WebServiceDefinitionError = errors.New("WebServiceDefinitionError")
var TimeOutError = errors.New("TimeOut")

const httpStatusErrorPrefix = "API responded with "

type HttpStatusError struct {
	HttpStatus int
}

func (h *HttpStatusError) Error() string {
	return httpStatusErrorPrefix + string(rune(h.HttpStatus))
}

type UrlAssembler interface {
	InfoEndpoint(environment Environment, definition WebServiceDefinition) string
}

type ServiceInstance struct {
	UrlAssembler    UrlAssembler
	Definition      WebServiceDefinition
	Environment     Environment
	Status          *ServiceStatus
	StatusLoadError error
	statusLoaded    bool
}

type Deployment struct {
	config Configuration
	Rows   []Row
}

type Row struct {
	Even  bool
	Name  string
	Cells []*DeploymentCell
}

type DeploymentCell struct {
	DeployedService *ServiceInstance
	Content         DeploymentCellContent
}

type DeploymentCellContent struct {
	Text    string
	Title   string
	IsError bool
}

type Configuration struct {
	Environments []Environment
	WebServices  []WebServiceDefinition
}

type Environment struct {
	Name    string
	BaseUrl string
}

type WebServiceDefinition struct {
	Name         string
	PathSelector string
}

type ServiceStatus struct {
	BuildInfo BuildInfo `json:"build"`
}

type BuildInfo struct {
	Version   string `json:"version"`
	BuildTime string `json:"buildTime"`
}

// constructs the infoendpoint by $env.BaseUrl+ $midfix + %serviceDefinition.path-selector + $postfix
type SimpleUrlConstructor struct {
	PostFix string
	MidFix  string
}

func (suc SimpleUrlConstructor) InfoEndpoint(environment Environment, definition WebServiceDefinition) string {
	return environment.BaseUrl + suc.MidFix + definition.PathSelector + suc.PostFix
}

// The status of tje n-th WebServiceDefinition will deployed in the m-th environment running the
// be found on position Rows[n-1].Cells[m-1]
type DeploymentOverview struct {
	// columns of the grid
	Environments []Environment
	// rows of the grid
	WebServices []WebServiceDefinition

	Rows []Row
}

func FileConfiguration(filename string) Configuration {
	file, _ := os.Open(filename)
	defer file.Close()
	decoder := json.NewDecoder(file)
	conf := Configuration{}
	err := decoder.Decode(&conf)
	if err != nil {
		log.Fatalln("Could not read config")
	}
	return conf
}

func NewDeployment(configuration Configuration, urlAssembler UrlAssembler) *Deployment {
	var services []ServiceInstance
	var rows []Row
	var index = 0
	for _, ws := range configuration.WebServices {
		var row = Row{Name: ws.Name, Even: index%2 == 0}
		index++
		for _, env := range configuration.Environments {
			var cell = new(DeploymentCell)
			deployedService := ServiceInstance{
				Environment:  env,
				Definition:   ws,
				UrlAssembler: urlAssembler,
			}
			cell.DeployedService = &deployedService
			row.Cells = append(row.Cells, cell)
			services = append(services, deployedService)
		}
		rows = append(rows, row)
	}
	return &Deployment{config: configuration, Rows: rows}
}

func (d Deployment) makeOverview() *DeploymentOverview {
	return &DeploymentOverview{
		Environments: d.config.Environments,
		WebServices:  d.config.WebServices,
		Rows:         d.Rows,
	}
}

func (d Deployment) fetchVersions() {

	waitGroup := new(sync.WaitGroup)
	for _, ws := range d.Rows {
		for _, cell := range ws.Cells {
			waitGroup.Add(1)
			// Asyncronous using Go Routines
			go func(finalCell *DeploymentCell, wg *sync.WaitGroup) {
				finalCell.updateCellContent(time.Second)
				wg.Done()
			}(cell, waitGroup)
		}
	}
	waitGroup.Wait()
	log.Print("Loaded all service informations")
}

func (d Deployment) createOverviewGrid() *DeploymentOverview {
	d.fetchVersions()
	return d.makeOverview()
}

func (d Deployment) WriteTable(wr io.Writer) error {
	return versionOverviewTemplate.Execute(wr, d.createOverviewGrid())
}

func (instance ServiceInstance) createKey() string {
	return instance.Environment.Name + "_" + instance.Definition.Name
}

func (ds *ServiceInstance) getStatus(timeout time.Duration) (*ServiceStatus, error) {
	infoEndpoint := ds.UrlAssembler.InfoEndpoint(ds.Environment, ds.Definition)
	spaceClient := http.Client{
		Timeout: timeout,
	}

	req, err := http.NewRequest(http.MethodGet, infoEndpoint, nil)
	if err != nil {
		log.Fatal(err)
		return nil, WebServiceDefinitionError
	}

	res, getErr := spaceClient.Do(req)
	if getErr != nil {
		log.Print("Error reading API "+infoEndpoint+": ", getErr)
		if strings.Contains(getErr.Error(), "request cancelled") {
			return nil, TimeOutError
		}
		return nil, getErr
	}

	if res.StatusCode != 200 {
		return nil, &HttpStatusError{HttpStatus: res.StatusCode}
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		log.Print(readErr)
		return nil, readErr
	}

	jsonErr := json.Unmarshal(body, &ds.Status)
	if jsonErr != nil {
		log.Print("Error unmarshalling info", jsonErr)
		return nil, jsonErr
	}

	ds.statusLoaded = true
	return ds.Status, nil
}

func (dc *DeploymentCell) updateCellContent(timeout time.Duration) {
	instance := dc.DeployedService
	status, err := instance.getStatus(timeout)
	if err == nil {
		key := instance.createKey()
		instance.Status = status
		log.Print(key + " --> " + status.BuildInfo.Version)
		dc.Content = DeploymentCellContent{
			Text:    status.BuildInfo.Version,
			Title:   status.BuildInfo.BuildTime,
			IsError: false,
		}
	} else {
		dc.Content = DeploymentCellContent{
			Text:    "??",
			Title:   err.Error(),
			IsError: true,
		}
	}
}
