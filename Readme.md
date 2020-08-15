This library creates an html table showing all versions of multiple web-services on multiple environments.

![Screenshot](https://github.com/DanielHons/web-service-overview/blob/master/doc/screenshot.png?raw=true)

The current implementation is only fetching version and build timestamp info endpoints providing a model like
```
{
  "build": {
    "buildTime": "2020-07-29 15:55:50.257 [Etc/UTC]",
    "version": "1.2.3"
  }
}
```

It is planned to extend this to be more configurable. 

###Disclaimer:
It is not recommeded to use this code inside of your projects in the current state.


###Usage example
```
package main

import (
	wso "github.com/DanielHons/web-service-overview"
	"log"
	"net/http"
)

const addr = ":8081"

var config wso.Configuration


func main() {
	config = wso.FileConfiguration("config.json")
	log.Print("http://localhost" + addr + "/versions")
	http.HandleFunc("/versions", showVersions)
	log.Fatal(http.ListenAndServe(addr, nil))
}



func showVersions(rw http.ResponseWriter, req *http.Request) {
	deployment := wso.NewDeployment(config, wso.SimpleUrlConstructor{
		PostFix: "/actuator/info",
		MidFix:  "/backend/",
	})
	err := deployment.WriteTable(rw)
	if err != nil {
		log.Panic(err)
	}
}

```


###Configuration
The deployment object has a property `InfoEndpointTimeout` defaulting to 2 seconds.
```
{
  "WebServices": [
    {
      "Name": "IOT Adapter",
      "PathSelector": "bridge/iot"
    },
    {
      "Name": "WeatherService",
      "PathSelector": "weather/current"
    },
    {
      "Name": "IdentityManager",
      "PathSelector": "identities"
    },
    {
      "Name": "EmailService",
      "PathSelector": "email-service"
    },
    {
      "Name": "NotificationBridge",
      "PathSelector": "bridge/notifications"
    },
    {
      "Name": "MobileAgentGateway",
      "PathSelector": "mag"
    },
    {
      "Name": "UserService",
      "PathSelector": "user-service"
    }
  ],
  "Environments": [
    {
      "Name": "Alpha",
      "BaseUrl": "https://staging.myapp.example/alpha"
    },
    {
      "Name": "Beta",
      "BaseUrl": "https://staging.myapp.example/beta"
    }
  ]
}
```

The info endpoint for the WeatherService on beta would in this case be
`https://staging.myapp.example/beta/backend/weather/current/actuator/info`