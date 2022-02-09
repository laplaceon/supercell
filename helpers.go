package main

import (
	"encoding/json"
	// "fmt"
	"github.com/PuerkitoBio/goquery"
	"net/http"
	"net/url"
	"strings"
)

func GetDocument(url string) *goquery.Document {
	request, _ := http.NewRequest("GET", url, nil)
	request.Header.Set("User-Agent", userAgent)
	
	resp, _ := client.Do(request)
	defer resp.Body.Close()

	doc, _ := goquery.NewDocumentFromResponse(resp)
	
	return doc
}

func GetJson(url string) interface{} {
	request, _ := http.NewRequest("GET", url, nil)
	request.Header.Set("User-Agent", userAgent)
	
	resp, _ := client.Do(request)
	defer resp.Body.Close()
	
	var f interface{}
	
	json.NewDecoder(resp.Body).Decode(&f)

	return f
}

func PostDocument(url string, form url.Values) *goquery.Document {
	request, _ := http.NewRequest("POST", url, strings.NewReader(form.Encode()))
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	
	resp, _ := client.Do(request)

	doc, _ := goquery.NewDocumentFromResponse(resp)
	
	return doc
}

func reverse(items []Episode) []Episode {
	for i := 0; i < len(items)/2; i++ {
		j := len(items) - i - 1
		items[i], items[j] = items[j], items[i]
	}
	return items
}