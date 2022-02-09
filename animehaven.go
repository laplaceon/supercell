package main

import (
	"fmt"
	"github.com/Machiel/slugify"
	"github.com/PuerkitoBio/goquery"
	"gopkg.in/mgo.v2/bson"
	"net/url"
	"strings"
)

func StartAH() {
	if sources["animehaven"].Ready && sources["animehaven"].Enabled {
		CycleAHDirectory(1)
		ScrapeAHRankings()
	}
}

func CycleAHDirectory(page int) {
	pageStr := fmt.Sprintf("%v", page)
	form := url.Values{}
    form.Add("action", "infinite_scroll")
    form.Add("genre", "all")
    form.Add("letter", "all")
    form.Add("ptype", "all")
    form.Add("year_id", "all")
    form.Add("ss", "")
    form.Add("sortby", "name")
    form.Add("next_page", pageStr)
	directoryUrl := "http://animehaven.to/wp-admin/admin-ajax.php"
	fmt.Println("Scraping directory page: " + directoryUrl + " page " + pageStr)
	doc := PostDocument(directoryUrl, form)
	series := doc.Find("article h2.entry-title a")
	if series.Length() != 0 {
		series.Each(func(i int, s *goquery.Selection) {
			link, _ := s.Attr("href")
			SendWork("animehaven", link, true)
		})
		page++
		CycleAHDirectory(page)
	} else {
		fmt.Printf("%v\n", doc.Text())
	}
}

func ScrapeAH(link string) {
	source := "animehaven"
	urlParts := strings.Split(link, "/")
	id := slugify.Slugify(urlParts[len(urlParts)-1])
	results := []Series{}
	seriesCollection.Find(bson.M{"source": source, "id": id}).All(&results)
	updatable := true
	if len(results) != 0 {
		result := results[0]
		if result.Completed {
			updatable = false
		}
	}
	if updatable {
		GetDocument(link)
		// doc := GetDocument(link)
		// series := Series{
			// Summary: doc.Find("div.synopsys").Text(),
		// }
		// fmt.Printf("%v\n", series)
	}
}

func ScrapeAHRankings() {
}