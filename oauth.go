package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"time"
)

const (
	oauthBaseURL = "https://www.flickr.com/services/oauth/"
)

type oauthRequest map[string]string

func (oa *oauthRequest) Execute(method string, result interface{}) error {
	addr, err := url.Parse(oauthBaseURL + method)
	if err != nil {
		log.Fatal(err)
	}

	q := url.Values{}
	q.Set("oauth_nonce", fmt.Sprintf("%d", rand.Int63()))
	q.Set("oauth_timestamp", fmt.Sprintf("%d", time.Now().Unix()))
	q.Set("oauth_consumer_key", config.ApiKey)
	q.Set("oauth_signature_method", "HMAC-SHA1")
	q.Set("oauth_version", "1.0")
	q.Set("oauth_callback", oauthCallbackListenURL())

	for k, v := range *oa {
		q.Set(k, v)
	}
	addr.RawQuery = q.Encode()

	//oauth_signature=7w18YS2bONDPL%2FzgyzP5XTr5af4%3D
	q.Set("oauth_signature", authSign(addr))
	addr.RawQuery = q.Encode()

	if verbose {
		fmt.Println(addr.String())
	}

	return err
}

func authSign(addr *url.URL) string {
	baseString := "GET&"
	baseString += url.QueryEscape(addr.String())

	h := hmac.New(sha1.New, []byte(config.ApiSecret))
	h.Write([]byte(baseString))
	binSig := h.Sum(nil)
	return base64.URLEncoding.EncodeToString(binSig)
}

func oauthCallbackListenURL() string {
	return "http://127.0.0.1:20222/oauth"
}
