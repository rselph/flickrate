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
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"time"
)

const (
	configFile    = ".flickrate"
	cacheFile     = ".flickrate_cache"
	restEndpoint  = "https://api.flickr.com/services/rest/"
	secondsPerDay = 60 * 60 * 24
)

var (
	authName  string
	apikey    string
	apisecret string

	targetName string
	minDays    int64
	maxDays    int64
	minViews   int64
	show       int
	openUrl    bool

	verbose      bool
	refresh      bool
	noCache      bool
	cache        photoCache
	cacheTouched bool

	configPath string
	cachePath  string
)

var config struct {
	AuthUser     string
	AuthUserNsId string

	ApiKey    string
	ApiSecret string

	OauthToken       string
	OauthTokenSecret string
}

func main() {
	flag.StringVar(&authName, "user", "", "your flickr user name")
	flag.StringVar(&apikey, "key", "", "flickr API key")
	flag.StringVar(&apisecret, "secret", "", "flickr API secret")
	flag.Int64Var(&minDays, "mindays", 60, "minimum age in days")
	flag.Int64Var(&maxDays, "maxdays", 3650, "maximum age in days")
	flag.Int64Var(&minViews, "minviews", 1000, "mimimum views")
	flag.IntVar(&show, "top", 10, "show top n photos")
	flag.BoolVar(&verbose, "v", false, "verbose")
	flag.BoolVar(&refresh, "refresh", false, "refresh login credentials")
	flag.BoolVar(&noCache, "nocache", false, "fetch new data, even if cache file is recent")
	flag.BoolVar(&openUrl, "o", false, "Open photo URLs in the browser")
	flag.Parse()

	usr, err := user.Current()
	if err != nil {
		log.Fatal(err)
	}

	configPath = filepath.Join(usr.HomeDir, configFile)
	cachePath = filepath.Join(usr.HomeDir, cacheFile)

	configBytes, err := ioutil.ReadFile(configPath)
	if err == nil {
		err = json.Unmarshal(configBytes, &config)
		if err != nil {
			log.Fatal(err)
		}
	}

	if refresh {
		config.OauthToken = ""
		config.OauthTokenSecret = ""
	}

	newConfig := false
	if authName != "" {
		if authName != config.AuthUser {
			newConfig = true
			config.AuthUser = authName
			config.OauthToken = ""
			config.OauthTokenSecret = ""
		}
	}
	if apikey != "" {
		newConfig = true
		config.ApiKey = apikey
	}
	if apisecret != "" {
		newConfig = true
		config.ApiSecret = apisecret
	}

	if config.AuthUser != "" && config.OauthTokenSecret == "" {
		err = authorizeUser()
		if err != nil {
			log.Fatal(err)
		}
		newConfig = true
	}

	if newConfig {
		writeConfig()
	}

	if config.OauthTokenSecret != "" {
		err = testUser()
		if err != nil {
			log.Fatal(err)
		}
	}

	targetName = flag.Arg(0)
	if targetName == "" {
		targetName = config.AuthUser
	}

	if targetName == "" {
		log.Fatal("Must supply a user name to query.")
	}

	doTheThing()
}

func writeConfig() {
	configBytes, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		log.Fatal(err)
	}
	ioutil.WriteFile(configPath, configBytes, 0600)
	if err != nil {
		log.Fatal(err)
	}
}

func doTheThing() {
	userId := getUserId()

	photos := getPhotos(userId)

	loadCache()
	allPhotos := getDetails(photos)
	err := saveCache()
	if err != nil {
		log.Fatal(err)
	}

	somePhotos := filterPhotos(allPhotos)

	sortPhotos(somePhotos)

	printPhotos(somePhotos)
}

type testUserResponse struct {
	User struct {
		Id       string `xml:"id,attr"`
		UserName struct {
			Value string `xml:",chardata"`
		} `xml:"username"`
	} `xml:"user"`
}

func testUser() (err error) {
	q := &flickrQuery{}
	resp := &testUserResponse{}
	err = q.Execute("flickr.test.login", resp)
	return
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
	if targetName == config.AuthUser && config.AuthUserNsId != "" {
		return config.AuthUserNsId
	}

	q := flickrQuery{"username": targetName}
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

	fmt.Printf("Found %d photos.\n", len(allPhotos))
	return allPhotos
}

type photoCache map[string]*photoInfo

func loadCache() {
	cache = photoCache{}
	if noCache {
		return
	}

	info, err := os.Stat(cachePath)
	if err != nil || info.ModTime().Before(time.Now().Add(-time.Hour)) {
		return
	}

	cacheBytes, err := ioutil.ReadFile(cachePath)
	if err != nil {
		log.Print(err)
		return
	}
	err = json.Unmarshal(cacheBytes, &cache)
	if err != nil {
		log.Print(err)
		return
	}
}

func saveCache() error {
	if !cacheTouched {
		return nil
	}

	cacheBytes, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	return ioutil.WriteFile(cachePath, cacheBytes, 0600)
}

type photoInfo struct {
	Id     string  `xml:"id,attr",json:"id"`
	Secret string  `xml:"secret,attr",json:"secret"`
	Views  int64   `xml:"views,attr",json:"views"`
	Rate   float64 `json:"rate"`
	age    int64
	Dates  struct {
		Posted           int64  `xml:"posted,attr",json:"posted"`
		Taken            string `xml:"taken,attr",json:"taken"`
		Takengranularity int    `xml:"takengranularity,attr",json:"takengranularity"`
		LastUpdate       int64  `xml:"lastupdate,attr",json:"lastupdate"`
	} `xml:"dates",json:"dates"`
	Urls struct {
		Values []struct {
			Type  string `xml:"type,attr",json:"type"`
			Value string `xml:",chardata",json:"value"`
		} `xml:"url",json:"url"`
	} `xml:"urls",json:"urls"`
}

type getInfoResponse struct {
	Photo photoInfo `xml:"photo"`
}

func getDetails(photos []photoPtr) []*photoInfo {
	now := time.Now().Unix()
	infos := make([]*photoInfo, len(photos))
	for i, photo := range photos {
		var ok bool
		infos[i], ok = cache[photo.Id]
		if ok {
			infos[i].age = now - infos[i].Dates.Posted
			continue
		}

		q := flickrQuery{"photo_id": photo.Id}
		info := &getInfoResponse{}
		err := q.Execute("flickr.photos.getInfo", info)
		if err != nil {
			log.Fatal(err)
		}
		if info.Photo.Views != 0 {
			info.Photo.age = now - info.Photo.Dates.Posted
			info.Photo.Rate = float64(info.Photo.Views) / float64(info.Photo.age)
		}
		infos[i] = &info.Photo
		cache[photo.Id] = &info.Photo
		cacheTouched = true
		if i != 0 && i%80 == 0 {
			fmt.Println()
		}
		fmt.Print(".")
	}

	fmt.Println()
	return infos
}

func filterPhotos(photos []*photoInfo) []*photoInfo {
	filtered := make([]*photoInfo, 0, len(photos))
	for _, p := range photos {
		if p.age/secondsPerDay >= minDays && p.age/secondsPerDay <= maxDays && p.Views >= minViews {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

type photoList []*photoInfo

func (a photoList) Len() int {
	return len(a)
}
func (a photoList) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a photoList) Less(i, j int) bool {
	return a[i].Rate > a[j].Rate
}

func sortPhotos(photos []*photoInfo) {
	sort.Stable(photoList(photos))
}

func printPhotos(photos []*photoInfo) {
	for i := 0; i < show && i < len(photos); i++ {
		p := *photos[i]
		fmt.Printf("%s\t% 5d\t%8.3f\t%s\n",
			time.Unix(p.Dates.Posted, 0).String(),
			p.Views,
			p.Rate*secondsPerDay,
			p.Urls.Values[0].Value)
		if openUrl {
			openInBrowser(p.Urls.Values[0].Value)
		}
	}
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
		log.Fatal(err)
	}
	q := &url.Values{}
	q.Set("method", method)
	q.Set("api_key", config.ApiKey)
	q.Set("format", "rest")
	for k, v := range *fq {
		q.Set(k, v)
	}
	if config.OauthTokenSecret != "" {
		authSign(addr, q, config.OauthTokenSecret)
	} else {
		addr.RawQuery = q.Encode()
	}

	if verbose {
		fmt.Printf("%s\n", addr.String())
		fmt.Println()
	}

	resp, err := http.Get(addr.String())
	if err != nil {
		return err
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}

	if verbose {
		fmt.Println(string(respBody))
		fmt.Println()
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s\n\t--> %d: %s", addr.String(), resp.StatusCode, resp.Status)
	}

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
