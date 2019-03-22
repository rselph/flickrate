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
	staleTime     = time.Hour
)

var (
	authName  string
	apikey    string
	apisecret string

	targetName   string
	minDays      int64
	maxDays      int64
	minViews     int64
	onlyTotal    bool
	onlyRate     bool
	onlyFaves    bool
	onlyFaveRate bool
	show         int
	openUrl      bool
	workers      int

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
	flag.BoolVar(&onlyTotal, "views", false, "only select by total views")
	flag.BoolVar(&onlyRate, "rate", false, "only select by view rate")
	flag.BoolVar(&onlyFaves, "faves", false, "only select by total favorites")
	flag.BoolVar(&onlyFaveRate, "faverate", false, "only select by faves per view")
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

	switch {
	case onlyRate:
		sortByRate(somePhotos)
	case onlyTotal:
		sortByViews(somePhotos)
	case onlyFaves:
		sortByFaves(somePhotos)
	case onlyFaveRate:
		sortByFaveRate(somePhotos)
	default:
		sortByViews(somePhotos)
		sortByFaves(somePhotos)
		sortByFaveRate(somePhotos)
		sortByRate(somePhotos)
	}

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
		Page      int         `xml:"page,attr"`
		Pages     int         `xml:"pages,attr"`
		PerPage   int         `xml:"perpage,attr"`
		Total     int         `xml:"total,attr"`
		PhotoList []*photoPtr `xml:"photo"`
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

func getPhotos(userId string) []*photoPtr {
	page := 1
	pages := 1
	allPhotos := make([]*photoPtr, 0)
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

type getFavesResponse struct {
	Photo struct {
		Total int64 `xml:"total,attr"`
	} `xml:"photo"`
}

func getFavesCount(photoId string) int64 {
	q := flickrQuery{
		"photo_id": photoId,
		"per_page": "1",
	}
	resp := &getFavesResponse{}
	err := q.Execute("flickr.photos.getFavorites", resp)
	if err != nil {
		log.Fatal(err)
	}

	return resp.Photo.Total
}

type photoCache map[string]*photo

func loadCache() {
	cache = photoCache{}
	if noCache {
		return
	}

	info, err := os.Stat(cachePath)
	if err != nil || info.ModTime().Before(time.Now().Add(-staleTime)) {
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
	Id     string `xml:"id,attr"`
	Secret string `xml:"secret,attr"`
	Views  int64  `xml:"views,attr"`
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
		} `xml:"url",json:"url"`
	} `xml:"urls"`

	TotalFaves int64
}

func (info *photo) age() int64 {
	return now - info.Info.Dates.Posted
}

func (info *photo) rate() float64 {
	age := info.age()
	if age != 0 {
		return float64(info.Info.Views) / float64(age)
	}
	return 0.0
}

func (info *photo) faveRate() float64 {
	if info.Info.Views != 0 {
		return float64(info.Info.TotalFaves) / float64(info.Info.Views)
	}

	return 0.0
}

type getInfoResponse struct {
	Photo photoInfo `xml:"photo"`
}

type photo struct {
	Ptr         *photoPtr
	Info        *photoInfo
	selected    bool
	LastFetched time.Time
}

func getDetails(photos []*photoPtr) []*photo {
	notCached := make([]*photoPtr, 0, len(photos))
	results := make([]*photo, 0, len(photos))
	for _, photo := range photos {
		tmp, ok := cache[photo.Id]
		if ok && tmp.LastFetched.After(time.Now().Add(-staleTime)) {
			results = append(results, tmp)
		} else {
			notCached = append(notCached, photo)
		}
	}
	cacheTouched = len(notCached) != 0

	jobs := make(chan *photoPtr)
	lookedUp := make(chan *photo)

	go func() {
		for _, job := range notCached {
			jobs <- job
		}
		close(jobs)
	}()

	collectorWg := sync.WaitGroup{}
	collectorWg.Add(1)
	go func() {
		defer collectorWg.Done()
		for result := range lookedUp {
			results = append(results, result)
			cache[result.Info.Id] = result
		}
	}()

	workerWg := sync.WaitGroup{}
	for i := 0; i < workers; i++ {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for job := range jobs {
				q := flickrQuery{"photo_id": job.Id}
				info := &getInfoResponse{}
				err := q.Execute("flickr.photos.getInfo", info)
				if err != nil {
					log.Fatal(err)
				}
				info.Photo.TotalFaves = getFavesCount(job.Id)

				p := &photo{Ptr: job, Info: &info.Photo, LastFetched: time.Now()}
				lookedUp <- p
			}
		}()
	}

	workerWg.Wait()
	close(lookedUp)
	collectorWg.Wait()

	return results
}

func filterPhotos(photos []*photo) []*photo {
	filtered := make([]*photo, 0, len(photos))
	for _, p := range photos {
		if p.age()/secondsPerDay >= minDays && p.age()/secondsPerDay <= maxDays && p.Info.Views >= minViews {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

func sortByRate(photos []*photo) {
	sort.SliceStable(photos, func(i, j int) bool {
		return photos[i].rate() > photos[j].rate()
	})
	for i := 0; i < show && i < len(photos); i++ {
		photos[i].selected = true
	}
}

func sortByViews(photos []*photo) {
	sort.SliceStable(photos, func(i, j int) bool {
		return photos[i].Info.Views > photos[j].Info.Views
	})
	for i := 0; i < show && i < len(photos); i++ {
		photos[i].selected = true
	}
}

func sortByFaves(photos []*photo) {
	sort.SliceStable(photos, func(i, j int) bool {
		return photos[i].Info.TotalFaves > photos[j].Info.TotalFaves
	})
	for i := 0; i < show && i < len(photos); i++ {
		photos[i].selected = true
	}
}

func sortByFaveRate(photos []*photo) {
	sort.SliceStable(photos, func(i, j int) bool {
		return photos[i].faveRate() > photos[j].faveRate()
	})
	for i := 0; i < show && i < len(photos); i++ {
		photos[i].selected = true
	}
}

func printPhotos(photos []*photo) {
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 4, 0, 2, ' ', 0)

	fmt.Fprint(w, "Date\tViews\tFaves\tRate\tTitle\tURL\t\n")
	fmt.Fprint(w, "-----\t-----\t-----\t-----\t-----\t-----\t\n")
	n := 0
	for _, p := range photos {
		if p.selected {
			n++
			fmt.Fprintf(w, "%s\t%6d\t%6d\t%5.1f\t%s\t%s\t\n",
				time.Unix(p.Info.Dates.Posted, 0).Format("2006-01-02"),
				p.Info.Views,
				p.Info.TotalFaves,
				p.rate()*secondsPerDay,
				Contract(p.Ptr.Title, 40, 8),
				p.Info.Urls.Values[0].Value)
			if openUrl {
				openInBrowser(p.Info.Urls.Values[0].Value)
			}
		}
	}
	w.Flush()
	fmt.Printf("Selected %d photos.\n\n", n)
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
