package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"time"
)

const (
	oauthBaseURL = "https://www.flickr.com/services/oauth/"
)

var (
	srv            *http.Server
	oauth_verifier = make(chan string)
)

type oauthRequest map[string]string

func authorizeUser() error {
	_, err := getToken()
	if err != nil {
		return err
	}

	err = getUserAuth()
	return err
}

func getToken() (secret string, err error) {
	q := &oauthRequest{"oauth_callback": oauthCallbackListenURL()}
	result, err := q.Execute("request_token", "")
	if err != nil {
		return
	}

	config.AuthToken = result.Get("oauth_token")
	secret = result.Get("oauth_token_secret")

	if result.Get("oauth_callback_confirmed") != "true" {
		err = fmt.Errorf("oauth_callback_confirmed was not true: %s", result.Get("oauth_callback_confirmed"))
		return
	}

	if verbose {
		fmt.Printf("token: %s\ntoken secret: %s\n", config.AuthToken, secret)
	}

	return
}

func getUserAuth() error {
	addr, err := url.Parse(oauthBaseURL + "authorize")
	if err != nil {
		log.Fatal(err)
	}

	q := &url.Values{}
	q.Set("oauth_token", config.AuthToken)
	q.Set("perms", "read")
	addr.RawQuery = q.Encode()

	err = openInBrowser(addr.String())
	if err != nil {
		log.Fatal(err)
	}

	config.AuthTokenVerifier = <-oauth_verifier
	if config.AuthTokenVerifier == "" {
		err = fmt.Errorf("Incomplete oauth authentication")
	}

	return err
}

func (oa *oauthRequest) Execute(method string, k string) (url.Values, error) {
	addr, err := url.Parse(oauthBaseURL + method)
	if err != nil {
		log.Fatal(err)
	}

	q := &url.Values{}
	q.Set("oauth_nonce", fmt.Sprintf("%d", rand.Int63()))
	q.Set("oauth_timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	q.Set("oauth_consumer_key", config.ApiKey)
	q.Set("oauth_signature_method", "HMAC-SHA1")
	q.Set("oauth_version", "1.0")

	for k, v := range *oa {
		q.Set(k, v)
	}

	q.Set("oauth_signature", authSign(addr, q, k))
	addr.RawQuery = q.Encode()

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

func authSign(addr *url.URL, q *url.Values, k string) string {
	baseString := "GET&"
	baseString += url.QueryEscape(addr.String())
	baseString += "&"
	baseString += url.QueryEscape(q.Encode())
	if verbose {
		fmt.Println(baseString)
	}

	h := hmac.New(sha1.New, []byte(config.ApiSecret+"&"+k))
	h.Write([]byte(baseString))
	binSig := h.Sum(nil)
	return base64.StdEncoding.EncodeToString(binSig)
}

type oauthResponseHandler struct{}

func (oah *oauthResponseHandler) ServeHTTP(resp http.ResponseWriter, req *http.Request) {
	if verbose {
		fmt.Println(req.URL.String())
	}
	req.Body.Close()
	go srv.Shutdown(context.Background())
	resp.Write([]byte("OK"))

	q := req.URL.Query()
	oauth_verifier <- q.Get("oauth_verifier")
}

func oauthCallbackListenURL() string {
	srv = &http.Server{
		Handler: &oauthResponseHandler{},
	}
	listen, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	go srv.Serve(listen)

	return fmt.Sprintf("http://%s/oauth", listen.Addr().String())
}

func openInBrowser(url string) (err error) {
	cmd := exec.Command("cmd.exe", "/C", "start", url)
	if verbose {
		fmt.Println(cmd.Args)
	}
	cmd.Run()
	return
}
