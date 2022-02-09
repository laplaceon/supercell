package main

import (
	"fmt"
	"github.com/Machiel/slugify"
	"github.com/PuerkitoBio/goquery"
	"gopkg.in/mgo.v2/bson"
	"strings"
)

func StartGG() {
	if sources["gogoanime"].Ready && sources["gogoanime"].Enabled {
		CycleGGDirectory(1)
		ScrapeGGRankings()
		ScrapeGGGenres()
	}
	wg.Done()
}

func CycleGGDirectory(page int) {
	directoryUrl := "http://ww1.gogoanime.io/anime-list.html?page=" + fmt.Sprintf("%v", page)
	fmt.Println("Scraping directory page: " + directoryUrl)
	doc := GetDocument(directoryUrl)
	doc.Find("div.anime_list_body li").Each(func(i int, s *goquery.Selection) {
		link, _ := s.Find("a").Attr("href")
		SendWork("gogoanime", "http://ww1.gogoanime.io" + link, false)
	})
	lastPage := doc.Find("ul.pagination-list li").Last()
	if !lastPage.HasClass("selected") {
		page++
		CycleGGDirectory(page)
	}
}

func ScrapeGG(link string) {
	source := "gogoanime"
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
		header := doc.Find("div.anime_info_body_bg")
		image, _ := header.Find("img").First().Attr("src")
		genres := []string{}
		doc.Find("p.type:has(span:contains(Genre)) a").Each(func(i int, s *goquery.Selection) {
			genre, _ := s.Attr("title")
			genres = append(genres, strings.ToLower(genre))
		})
		series := Series{
			Source: source,
			Id: id,
			Url: link,
			Title: header.Find("h1").First().Text(),
			Image: image,
			Summary: strings.TrimSpace(strings.Replace(doc.Find("p.type:has(span:contains(Plot))").Text(), "Plot Summary:", "", 1)),
			Genres: genres,
			Completed: strings.Contains(doc.Find("p.type:has(span:contains(Status))").Text(), "Completed"),
		}
		sid, _ := doc.Find("input#movie_id").Attr("value")
		episodesDoc := GetDocument("http://ww1.gogoanime.io//load-list-episode?ep_start=0&ep_end=1000&id=" + sid + "&default_ep=0")
		episodes := []Episode{}
		episodesDoc.Find("ul#episode_related a").Each(func(i int, s *goquery.Selection) {
			name := strings.Split(s.Find("div.name").Text(), " ")
			link, _ := s.Attr("href")
			episodes = append(episodes, Episode{Id: name[len(name)-1], Link: "http://ww1.gogoanime.io" + strings.TrimSpace(link)})
		})
		listing := Listing{
			Name: "",
			Episodes: reverse(episodes),
		}
		episodesData := Episodes{
			Source: source,
			Id: id,
			Listings: []Listing{listing},
		}
		
		seriesCollection.Upsert(bson.M{"source": source, "id": id}, series)
		_, episodesErr := episodesCollection.Upsert(bson.M{"source": source, "id": id}, episodesData)
		
		if episodesErr != nil {
			db.Create(&Log{Type: 2, Status: 2, Message: "Failed to update episodes for " + source + " / " + id})
		}
		
		fmt.Println("Inserted source: " + source + " id: " + id)
	} else {
		fmt.Println("Source: " + source + " id: " + id + " not added. Already complete")
	}
}

func ScrapeGGRankings() {
	doc := GetDocument("http://ww1.gogoanime.io/popular.html")
	popularRankingIds := []string{}
	doc.Find("div.last_episodes ul.items div.img > a").Each(func(i int, s *goquery.Selection) {
		link, _ := s.Attr("href")
		urlParts := strings.Split(link, "/")
		popularRankingIds = append(popularRankingIds, slugify.Slugify(urlParts[len(urlParts)-1]))
	})
	popularRanking := Ranking{
		Source: "gogoanime",
		Ranking: "popular",
		Series: popularRankingIds,
	}
	rankingsCollection.Upsert(bson.M{"source": "gogoanime", "ranking": "popular"}, popularRanking)
	doc = GetDocument("http://ww1.gogoanime.io//page-recent-release.html?page=1&type=1")
	newestRankingIds := []string{}
	doc.Find("div.last_episodes ul.items div.img > a").Each(func(i int, s *goquery.Selection) {
		link, _ := s.Attr("href")
		urlParts := strings.Split(link, "/")
		id := slugify.Slugify(urlParts[len(urlParts)-1])
		strings.Split(id, "-episode")
		newestRankingIds = append(newestRankingIds, strings.Split(id, "-episode")[0])
	})
	newestRanking := Ranking{
		Source: "gogoanime",
		Ranking: "newest",
		Series: newestRankingIds,
	}
	rankingsCollection.Upsert(bson.M{"source": "gogoanime", "ranking": "newest"}, newestRanking)
}

func ScrapeGGGenres() {
	genres := []string{}
	doc := GetDocument("http://www.masterani.me/anime")
	doc.Find("div.genres-pop-out div.item").Each(func(i int, s *goquery.Selection) {
		genres = append(genres, strings.ToLower(strings.TrimSpace(s.Text())))
	})
	extrasCollection.Upsert(bson.M{"id": "gogoanime", "key": "genres"}, bson.M{"$set": bson.M{"data": genres}})
}