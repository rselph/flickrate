package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1" // #nosec Require insecure hash for Flickr API
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	oauthBaseURL = "https://www.flickr.com/services/oauth/"
)

var (
	srv                  *http.Server
	oauthVerifierChannel = make(chan string)

	oauthTokenStage1       string
	oauthTokenSecretStage1 string
	oauthVerifier          string
)

type oauthRequest map[string]string

func authorizeUser() error {
	err := getToken()
	if err != nil {
		return err
	}

	err = getUserAuth()
	if err != nil {
		return err
	}

	err = getAccessTokenSecret()
	return err
}

func getToken() (err error) {
	q := &oauthRequest{"oauth_callback": oauthCallbackListenURL()}
	result, err := q.Execute("request_token", "")
	if err != nil {
		return
	}

	oauthTokenStage1 = result.Get("oauth_token")
	oauthTokenSecretStage1 = result.Get("oauth_token_secret")

	if result.Get("oauth_callback_confirmed") != "true" {
		err = fmt.Errorf("oauth_callback_confirmed was not true: %s", result.Get("oauth_callback_confirmed"))
		return
	}

	return
}

func getUserAuth() error {
	addr, err := url.Parse(oauthBaseURL + "authorize")
	if err != nil {
		log.Fatal(err)
	}

	q := &url.Values{}
	q.Set("oauth_token", oauthTokenStage1)
	q.Set("perms", "read")
	addr.RawQuery = q.Encode()

	err = openInBrowser(addr.String())
	if err != nil {
		log.Fatal(err)
	}

	oauthVerifier = <-oauthVerifierChannel
	if oauthVerifier == "" {
		err = fmt.Errorf("incomplete oauth authentication")
	}

	return err
}

func getAccessTokenSecret() (err error) {
	q := &oauthRequest{
		"oauth_verifier": oauthVerifier,
		"oauth_token":    oauthTokenStage1,
	}

	result, err := q.Execute("access_token", oauthTokenSecretStage1)
	if err != nil {
		return
	}

	config.OauthToken = result.Get("oauth_token")
	config.OauthTokenSecret = result.Get("oauth_token_secret")
	config.AuthUserNsId = result.Get("user_nsid")

	if verbose {
		fmt.Printf("token: %s\ntoken secret: %s\n", config.OauthToken, config.OauthTokenSecret)
	}

	return
}

func (oa *oauthRequest) Execute(method string, k string) (url.Values, error) {
	addr, err := url.Parse(oauthBaseURL + method)
	if err != nil {
		log.Fatal(err)
	}

	q := &url.Values{}
	for k, v := range *oa {
		q.Set(k, v)
	}

	authSign(addr, q, k)

	if verbose {
		fmt.Println(addr.String())
	}

	resp, err := http.Get(addr.String())
	if err != nil {
		return nil, err
	}

	respBody, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, err
	}

	if verbose {
		fmt.Println(string(respBody))
		fmt.Println()
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s\n\t--> %d: %s", addr.String(), resp.StatusCode, resp.Status)
	}

	result, err := url.ParseQuery(string(respBody))
	if err != nil {
		return nil, err
	}

	return result, err
}

func authSign(addr *url.URL, q *url.Values, k string) {
	addr.RawQuery = ""
	q.Set("oauth_nonce", fmt.Sprintf("%d", rand.Int63()))
	q.Set("oauth_timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	q.Set("oauth_consumer_key", config.ApiKey)
	q.Set("oauth_signature_method", "HMAC-SHA1")
	q.Set("oauth_version", "1.0")
	if config.OauthToken != "" {
		q.Set("oauth_token", config.OauthToken)
	}

	baseString := "GET&"
	baseString += url.QueryEscape(addr.String())
	baseString += "&"
	baseString += url.QueryEscape(q.Encode())
	if verbose {
		fmt.Println(baseString)
	}

	h := hmac.New(sha1.New, []byte(config.ApiSecret+"&"+k))
	_, _ = h.Write([]byte(baseString))
	binSig := h.Sum(nil)
	q.Set("oauth_signature", base64.StdEncoding.EncodeToString(binSig))
	addr.RawQuery = q.Encode()
}

type oauthResponseHandler struct{}

func (oah *oauthResponseHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if verbose {
		fmt.Println(req.URL.String())
	}
	req.Body.Close()
	go func() { _ = srv.Shutdown(context.Background()) }()
	_, _ = resp.Write([]byte("You can now close this browser window and return to flickrate."))

	q := req.URL.Query()
	oauthVerifierChannel <- q.Get("oauth_verifier")
}

func oauthCallbackListenURL() string {
	srv = &http.Server{
		Handler: &oauthResponseHandler{},
	}
	listen, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	go func() { _ = srv.Serve(listen) }()

	return fmt.Sprintf("http://%s/oauth", listen.Addr().String())
}
