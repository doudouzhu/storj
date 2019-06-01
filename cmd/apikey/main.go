package main

import (
	"fmt"
)

var (
	Host                           = "http://127.0.0.1:8081"
	CreateRegistrationTokenAddress = Host + "/registrationToken?projectsLimit=1"
	ConsoleActivationAddress       = Host + "/activation/?token="
	ConsoleAPIAddress              = Host + "/api/graphql/v0"
)

type Param struct {
	Token         string
	UserEmail     string
	UserPassword  string
	UserFullName  string
	UserShortName string
	ProjectName   string
	APIKeyName    string
}

var (
	p = &Param{
		Token:         "",
		UserEmail:     "",
		UserPassword:  "",
		UserShortName: "store",
		UserFullName:  "storj store",
		ProjectName:   "store",
		APIKeyName:    "TestProject",
	}
)

func main() {
	var err error
	var apiKey string

	if err = addProjectWithKey(&apiKey, CreateRegistrationTokenAddress, ConsoleActivationAddress, ConsoleAPIAddress, p); err != nil {
		fmt.Println("create project fail, err=%s", err)
	} else {
		fmt.Printf("create projcet succ, apiKey=%s\n", apiKey)
	}
}
