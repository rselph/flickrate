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

	photos := getPhotos(userId)
	fmt.Printf("%v\n", photos)

	details := getDetails(photos)
	fmt.Printf("%v\n", details)
}

type getUserIdResponse struct {
	User struct {
		Id       string `xml:"id,attr"`
		NsId     string `xml:"nsid,attr"`
		UserName struct {
			Value string `xml:",chardata"`
		} `xml:"username"`
	} `xml:"user"`
}

func getUserId() string {
	q := flickrQuery{"username": config.UserName}
	resp := &getUserIdResponse{}
	err := q.Execute("flickr.people.findByUsername", resp)
	if err != nil {
		log.Fatal(err)
	}

	return resp.User.NsId
}

type getPhotosResponse struct {
	Photos struct {
		Page      int        `xml:"page,attr"`
		Pages     int        `xml:"pages,attr"`
		PerPage   int        `xml:"perpage,attr"`
		Total     int        `xml:"total,attr"`
		PhotoList []photoPtr `xml:"photo"`
	} `xml:"photos"`
}

type photoPtr struct {
	Id       string `xml:"id,attr"`
	Owner    string `xml:"owner,attr"`
	Secret   string `xml:"secret,attr"`
	Server   string `xml:"server,attr"`
	Farm     string `xml:"farm,attr"`
	Title    string `xml:"title,attr"`
	IsPublic int    `xml:"ispublic,attr"`
	IsFriend int    `xml:"isfriend,attr"`
	IsFamily int    `xml:"isfamily,attr"`
}

func getPhotos(userId string) []photoPtr {
	page := 1
	pages := 1
	allPhotos := make([]photoPtr, 0)
	for page <= pages {
		q := flickrQuery{
			"user_id":     userId,
			"safe_search": "3",
			"page":        fmt.Sprintf("%d", page),
		}
		resp := &getPhotosResponse{}
		err := q.Execute("flickr.photos.search", resp)
		if err != nil {
			log.Fatal(err)
		}
		allPhotos = append(allPhotos, resp.Photos.PhotoList...)
		pages = resp.Photos.Pages
		page++
	}

	return allPhotos
}

type photoInfo struct {
	Id     string `xml:"id,attr"`
	Secret string `xml:"secret,attr"`
	Dates  struct {
		Posted           int64  `xml:"posted,attr"`
		Taken            string `xml:"taken,attr"`
		Takengranularity int    `xml:"takengranularity,attr"`
		LastUpdate       int64  `xml:"lastupdate,attr"`
	} `xml:"dates"`
	Urls struct {
		Values []struct {
			Type  string `xml:"type,attr"`
			Value string `xml:",chardata"`
		} `xml:"url"`
	} `xml:"urls"`
}

type getInfoResponse struct {
	Photo photoInfo `xml:"photo"`
}

func getDetails(photos []photoPtr) []*photoInfo {
	infos := make([]*photoInfo, len(photos))
	for i, photo := range photos {
		q := flickrQuery{"photo_id": photo.Id}
		info := &getInfoResponse{}
		err := q.Execute("flickr.photos.getInfo", info)
		if err != nil {
			log.Fatal(err)
		}
		infos[i] = &info.Photo
	}

	return infos
}

type flickrQuery map[string]string
type flickrError struct {
	Stat string `xml:"stat,attr"`
	Err  struct {
		Code string `xml:"code,attr"`
		Msg  string `xml:"msg,attr"`
	} `xml:"err"`
}

func (fq *flickrQuery) Execute(method string, result interface{}) error {
	addr, err := url.Parse(restEndpoint)
	if err != nil {
		return err
	}
	q := addr.Query()
	q.Set("method", method)
	q.Set("api_key", config.ApiKey)
	q.Set("format", "rest")
	for k, v := range *fq {
		q.Set(k, v)
	}
	addr.RawQuery = q.Encode()
	fmt.Printf("%s\n", addr.String())
	fmt.Println()

	resp, err := http.Get(addr.String())
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s\n\t--> %d: %s", addr.String(), resp.StatusCode, resp.Status)
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	fmt.Println(string(respBody))
	fmt.Println()

	fe := &flickrError{}
	err = xml.Unmarshal(respBody, fe)
	if err != nil {
		return err
	}
	if fe.Stat != "ok" {
		return fmt.Errorf("%s: %s", fe.Stat, fe.Err.Msg)
	}

	return xml.Unmarshal(respBody, result)
}
