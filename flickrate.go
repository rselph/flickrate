package main

import (
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"sync"
	"text/tabwriter"
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
	workers    int

	verbose      bool
	refresh      bool
	noCache      bool
	cache        photoCache
	cacheTouched bool
	now          int64

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
	flag.Int64Var(&maxDays, "maxdays", math.MaxInt64, "maximum age in days")
	flag.Int64Var(&minViews, "minviews", 1000, "mimimum views")
	flag.IntVar(&show, "top", 10, "show top n photos")
	flag.BoolVar(&verbose, "v", false, "verbose")
	flag.BoolVar(&refresh, "refresh", false, "refresh login credentials")
	flag.BoolVar(&noCache, "nocache", false, "fetch new data, even if cache file is recent")
	flag.BoolVar(&openUrl, "o", false, "Open photo URLs in the browser")
	flag.IntVar(&workers, "w", 20, "number of queries to make at a time")
	flag.Parse()

	now = time.Now().Unix()

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

	sortByViews(somePhotos)
	sortByRate(somePhotos)

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

	fileReader, err := os.Open(cachePath)
	if err != nil {
		log.Print(err)
		return
	}
	defer fileReader.Close()

	gzReader, err := gzip.NewReader(fileReader)
	if err != nil {
		log.Print(err)
		return
	}
	defer gzReader.Close()

	cacheBytes, err := ioutil.ReadAll(gzReader)
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

	fileWriter, err := os.Create(cachePath)
	if err != nil {
		return err
	}
	defer fileWriter.Close()

	gzWriter, err := gzip.NewWriterLevel(fileWriter, gzip.BestCompression)
	if err != nil {
		return err
	}
	defer gzWriter.Close()

	_, err = gzWriter.Write(cacheBytes)

	return err
}

type photoInfo struct {
	Id        string `xml:"id,attr"`
	Secret    string `xml:"secret,attr"`
	Views     int64  `xml:"views,attr"`
	fromCache bool
	selected  bool
	Dates     struct {
		Posted           int64  `xml:"posted,attr"`
		Taken            string `xml:"taken,attr"`
		Takengranularity int    `xml:"takengranularity,attr"`
		LastUpdate       int64  `xml:"lastupdate,attr"`
	} `xml:"dates"`
	Urls struct {
		Values []struct {
			Type  string `xml:"type,attr"`
			Value string `xml:",chardata"`
		} `xml:"url",json:"url"`
	} `xml:"urls"`
	Ptr photoPtr
}

func (info *photoInfo) age() int64 {
	return now - info.Dates.Posted
}

func (info *photoInfo) rate() float64 {
	age := info.age()
	if age != 0 {
		return float64(info.Views) / float64(age)
	}
	return 0.0
}

type getInfoResponse struct {
	Photo photoInfo `xml:"photo"`
}

func getDetails(photos []photoPtr) []*photoInfo {
	infos := make([]*photoInfo, 0, len(photos))
	jobs := make(chan *photoPtr)
	results := make(chan *photoInfo)

	jobWg := sync.WaitGroup{}
	for i := 0; i < workers; i++ {
		go func() {
			for job := range jobs {
				q := flickrQuery{"photo_id": job.Id}
				info := &getInfoResponse{}
				err := q.Execute("flickr.photos.getInfo", info)
				if err != nil {
					log.Fatal(err)
				}
				info.Photo.Ptr = *job
				results <- &info.Photo
				jobWg.Done()
			}
		}()
	}

	resultWg := sync.WaitGroup{}
	resultWg.Add(1)
	go func() {
		defer resultWg.Done()
		i := 0
		for info := range results {
			infos = append(infos, info)
			if i != 0 && i%80 == 0 {
				fmt.Println()
			}
			fmt.Print(".")
			i++
		}
		fmt.Println()
	}()

	for _, photo := range photos {
		tmp, ok := cache[photo.Id]
		if ok {
			tmp.fromCache = true
			results <- tmp
		} else {
			jobWg.Add(1)
			jobs <- &photo
		}
	}

	close(jobs)
	jobWg.Wait()

	close(results)
	resultWg.Wait()

	for _, info := range infos {
		if !info.fromCache {
			cache[info.Id] = info
			cacheTouched = true
		}
	}

	return infos
}

func filterPhotos(photos []*photoInfo) []*photoInfo {
	filtered := make([]*photoInfo, 0, len(photos))
	for _, p := range photos {
		if p.age()/secondsPerDay >= minDays && p.age()/secondsPerDay <= maxDays && p.Views >= minViews {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func sortByRate(photos []*photoInfo) {
	sort.SliceStable(photos, func(i, j int) bool {
		return photos[i].rate() > photos[j].rate()
	})
	for i := 0; i < show && i < len(photos); i++ {
		photos[i].selected = true
	}
}

func sortByViews(photos []*photoInfo) {
	sort.SliceStable(photos, func(i, j int) bool {
		return photos[i].Views > photos[j].Views
	})
	for i := 0; i < show && i < len(photos); i++ {
		photos[i].selected = true
	}
}

func printPhotos(photos []*photoInfo) {
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 4, 0, 2, ' ', 0)
	defer w.Flush()

	fmt.Fprint(w, "Date\tViews\tRate\tTitle\tURL\t\n")
	fmt.Fprint(w, "-----\t-----\t-----\t-----\t-----\t\n")
	for _, p := range photos {
		if p.selected {
			fmt.Fprintf(w, "%s\t%6d\t%5.1f\t%s\t%s\t\n",
				time.Unix(p.Dates.Posted, 0).Format("2006-01-02"),
				p.Views,
				p.rate()*secondsPerDay,
				Contract(p.Ptr.Title, 40, 8),
				p.Urls.Values[0].Value)
			if openUrl {
				openInBrowser(p.Urls.Values[0].Value)
			}
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
