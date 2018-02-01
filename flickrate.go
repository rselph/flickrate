package main

import (
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os/user"
	"path/filepath"
)

const (
	configFile   = ".flickrate"
	restEndpoint = "https://api.flickr.com/services/rest/"
)

var (
	username  string
	apikey    string
	apisecret string

	minDays  int
	minViews int
)

var config struct {
	UserName  string
	ApiKey    string
	ApiSecret string
}

func main() {
	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	flag.StringVar(&username, "user", usr.Name, "flickr user name")
	flag.StringVar(&apikey, "key", "", "flickr API key")
	flag.StringVar(&apisecret, "secret", "", "flickr API secret")
	flag.IntVar(&minDays, "days", 10, "minimum days")
	flag.IntVar(&minViews, "views", 1000, "mimimum views")
	flag.Parse()

	configPath := filepath.Join(usr.HomeDir, configFile)
	configBytes, err := ioutil.ReadFile(configPath)
	if err == nil {
		err = json.Unmarshal(configBytes, &config)
		if err != nil {
			log.Fatal(err)
		}
	}

	newConfig := false
	if username != "" {
		newConfig = true
		config.UserName = username
	}
	if apikey != "" {
		newConfig = true
		config.ApiKey = apikey
	}
	if apisecret != "" {
		newConfig = true
		config.ApiSecret = apisecret
	}

	if newConfig {
		configBytes, err = json.MarshalIndent(config, "", "    ")
		if err != nil {
			log.Fatal(err)
		}
		ioutil.WriteFile(configPath, configBytes, 0600)
		if err != nil {
			log.Fatal(err)
		}
	}

	doTheThing()
}

func doTheThing() {
	userId := getUserId()
	fmt.Println(userId)
}

type request struct {
	ApiKey string `xml:"api_key"`
	Format string `xml:"format"`
}

type userIdRequest struct {
	request
	UserName string `xml:"username"`
}

func getUserId() string {
	req := &userIdRequest{
		request:  request{config.ApiKey, "rest"},
		UserName: config.UserName,
	}
	reqBytes, err := xml.Marshal(req)
	if err != nil {
		log.Fatal(err)
	}
	reqBytes = append([]byte(xml.Header), reqBytes...)
	//reqReader := ioutil.NopCloser(bytes.NewReader(reqBytes))

	addr, err := url.Parse(restEndpoint)
	q := addr.Query()
	q.Set("method", "flickr.people.findByUsername")
	q.Set("api_key", req.ApiKey)
	q.Set("format", req.Format)
	q.Set("username", req.UserName)
	addr.RawQuery = q.Encode()
	fmt.Printf("%s\n", addr.String())
	fmt.Println()
	if err != nil {
		log.Fatal(err)
	}

	//fmt.Println(string(reqBytes))
	//fmt.Println()
	httpReq := &http.Request{}
	httpReq.Method = http.MethodGet
	httpReq.URL = addr
	//httpReq.Body = reqReader

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		errString := fmt.Sprintf("%s\n\t--> %d: %s", addr.String(), resp.StatusCode, resp.Status)
		log.Fatal(errString)
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	fmt.Println(string(respBody))
	fmt.Println()
	return ""
}
