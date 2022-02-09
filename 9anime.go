package main

import (
	"fmt"
	"github.com/Machiel/slugify"
	"github.com/PuerkitoBio/goquery"
	"gopkg.in/mgo.v2/bson"
	"strings"
	"time"
)

func Start9A() {
	if sources["9anime"].Ready && sources["9anime"].Enabled {
		Cycle9ADirectory(1)
		Scrape9ARankings()
		Scrape9AGenres()
	}
	wg.Done()
}

func Check9A(doc *goquery.Document) bool {
	title := doc.Find("title").First().Text()
	return strings.Contains(title, "403") || strings.Contains(title, "503")
}

func Sleep9A() {
	time.Sleep(time.Duration(1000) * time.Millisecond)
}

func Cycle9ADirectory(page int) {
	directoryUrl := "https://9anime.to/filter?sort=title:asc&page=" + fmt.Sprintf("%v", page)
	fmt.Println("Scraping directory page: " + directoryUrl)
	doc := GetDocument(directoryUrl)
	Sleep9A()
	if !Check9A(doc) {
		doc.Find("div.list-film div.item a.poster").Each(func(i int, s *goquery.Selection) {
			link, _ := s.Attr("href")
			SendWork("9anime", link, true)
		})
	}
	
	nextButton := doc.Find("a.btn.pull-right.disabled").Size()
	if nextButton == 0 {
		page++
		Cycle9ADirectory(page)
	}
}

func ListenScrape9A() {
	if len(lbJobs["9anime"]) > 0 {
		lbLock.Lock()
		top := lbJobs["9anime"][0]
		lbJobs["9anime"] = lbJobs["9anime"][1:]
		lbLock.Unlock()
		Scrape9A(top.Link)
		wg.Done()
	}
}

func Scrape9A(link string) {
	source := "9anime"
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
		doc := GetDocument(link)
		alternateTitles := strings.Split(doc.Find("dt:contains(Other) + dd").Text(), "; ")
		genres := []string{}
		doc.Find("dt:contains(Genre) + dd a").Each(func(i int, s *goquery.Selection) {
			genres = append(genres, strings.ToLower(s.Text()))
		})
		var summary string
		description := doc.Find("div.desc")
		if description.Has("div.shortcontent").Length() == 0 {
			summary = description.Text()
		} else {
			summary = description.Find("div.fullcontent").Text()
		}
		image, _ := doc.Find("div#info div.thumb img").First().Attr("src")
		series := Series{
			Source: source,
			Id: id,
			Url: link,
			Title: doc.Find("h1.title").Text(),
			Image: strings.SplitAfter(image, "&url=")[1],
			Summary: summary,
			Genres: genres,
			Completed: strings.Contains(doc.Find("dt:contains(Status) + dd").Text(), "Completed"),
		}
		if alternateTitles[0] != "" {
			series.AlternateTitles = alternateTitles
		}
		listings := []Listing{}
		doc.Find("div#servers div.server").Each(func(i int, s *goquery.Selection) {
			episodes := []Episode{}
			s.Find("ul.episodes li a").Each(func(j int, t *goquery.Selection) {
				link, _ := t.Attr("href")
				episodes = append(episodes, Episode{Name: strings.TrimLeft(t.Text(), "0"), Link: "http://9anime.to" + link})
			})
			listing := Listing{
				Name: strings.TrimSpace(s.Find("label.name").First().Text()),
				Episodes: episodes,
			}
			listings = append(listings, listing)
		})
		episodesData := Episodes{
			Source: source,
			Id: id,
			Listings: listings,
		}
		
		if series.Title == "" {
			fmt.Println("Failed")
		}
		
		seriesCollection.Upsert(bson.M{"source": source, "id": id}, series)
		_, episodesErr := episodesCollection.Upsert(bson.M{"source": source, "id": id}, episodesData)
		
		if episodesErr != nil {
			db.Create(&Log{Type: 2, Status: 2, Message: "Failed to update episodes for " + source + " / " + id})
		}
		
		fmt.Println("Inserted source: " + source + " id: " + id)
		Sleep9A()
	} else {
		fmt.Println("Source: " + source + " id: " + id + " not added. Already complete")
	}
}

func Scrape9ARankings() {
	doc := GetDocument("http://9anime.to/filter?sort=scores:desc")
	Sleep9A()
	popularRankingIds := []string{}
	doc.Find("div.list-film div.item a.poster").Each(func(i int, s *goquery.Selection) {
		link, _ := s.Attr("href")
		urlParts := strings.Split(link, "/")
		popularRankingIds = append(popularRankingIds, slugify.Slugify(urlParts[len(urlParts)-1]))
	})
	popularRanking := Ranking{
		Source: "9anime",
		Ranking: "popular",
		Series: popularRankingIds,
	}
	rankingsCollection.Upsert(bson.M{"source": "9anime", "ranking": "popular"}, popularRanking)
	doc = GetDocument("http://9anime.to/newest")
	Sleep9A()
	newestRankingIds := []string{}
	doc.Find("div.list-film div.item a.poster").Each(func(i int, s *goquery.Selection) {
		link, _ := s.Attr("href")
		urlParts := strings.Split(link, "/")
		newestRankingIds = append(newestRankingIds, slugify.Slugify(urlParts[len(urlParts)-1]))
	})
	newestRanking := Ranking{
		Source: "9anime",
		Ranking: "newest",
		Series: newestRankingIds,
	}
	rankingsCollection.Upsert(bson.M{"source": "9anime", "ranking": "newest"}, newestRanking)
}

func Scrape9AGenres() {
	genres := []string{}
	doc := GetDocument("http://9anime.to/")
	Sleep9A()
	doc.Find("ul#menu li:contains(Genre) > ul li a").Each(func(i int, s *goquery.Selection) {
		genres = append(genres, strings.ToLower(strings.TrimSpace(s.Text())))
	})
	extrasCollection.Upsert(bson.M{"id": "9anime", "key": "genres"}, bson.M{"$set": bson.M{"data": genres}})
}