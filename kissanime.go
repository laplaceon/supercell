package main

import (
	"fmt"
	"github.com/Machiel/slugify"
	"github.com/PuerkitoBio/goquery"
	"gopkg.in/mgo.v2/bson"
	"net/url"
	"strings"
)

func StartKA() {
	if sources["kissanime"].Ready && sources["kissanime"].Enabled {
		CycleKADirectory()
		ScrapeKARankings()
		ScrapeKAGenres()
	}
	wg.Done()
}

func CycleKADirectory() {
	form := url.Values{}
    form.Add("animeName", "")
    form.Add("genres", "0")
    form.Add("status", "")
	directoryUrl := "http://kissanime.ru/AdvanceSearch"
	fmt.Println("Scraping directory page: " + directoryUrl)
	doc := PostDocument(directoryUrl, form)
	doc.Find("table.listing td:nth-child(1) a").Each(func(i int, s *goquery.Selection) {
		link, _ := s.Attr("href")
		SendWork("kissanime", "http://kissanime.ru" + link, false)
	})
}

func ScrapeKA(link string) {
	source := "kissanime"
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
		image, _ := doc.Find("div.rightBox div.barContent img").First().Attr("src")
		genres := []string{}
		doc.Find("div.barContent p:has(span.info:contains(Genres)) a").Each(func(i int, s *goquery.Selection) {
			genres = append(genres, strings.ToLower(strings.TrimSpace(s.Text())))
		})
		alternateTitles := []string{}
		doc.Find("div.barContent p:has(span.info:contains(Other)) a").Each(func(i int, s *goquery.Selection) {
			alternateTitles = append(alternateTitles, s.Text())
		})
		title := doc.Find("div.barContent a.bigChar").Text()
		series := Series{
			Source: source,
			Id: id,
			Url: link,
			Title: title,
			AlternateTitles: alternateTitles,
			Image: image,
			Summary: doc.Find("div.barContent p:contains(Summary)").NextFiltered("p").Text(),
			Genres: genres,
			Completed: strings.Contains(doc.Find("div.barContent p:has(span.info:contains(Status))").Text(), "Completed"),
		}
		episodes := []Episode{}
		doc.Find("table.listing td:nth-child(1) a").Each(func(i int, s *goquery.Selection) {
			name := strings.TrimSpace(strings.Replace(s.Text(), title, "", 1))
			link, _ := s.Attr("href")
			episode := Episode{}
			if strings.Contains(name, "Episode") && strings.Contains(name, " - ") {
				parts := strings.Split(name, " - ")
				episode.Id = strings.TrimLeft(strings.TrimSpace(strings.Replace(parts[0], "Episode", "", 1)), "0")
				episode.Name = parts[1]
			} else {
				episode.Name = strings.TrimLeft(name, "_")
			}
			
			episode.Link = "http://kissanime.ru" + link
			
			episodes = append(episodes, episode)
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

func ScrapeKARankings() {
	doc := GetDocument("http://kissanime.ru/AnimeList/MostPopular")
	popularRankingIds := []string{}
	doc.Find("table.listing td:nth-child(1) a").Each(func(i int, s *goquery.Selection) {
		link, _ := s.Attr("href")
		urlParts := strings.Split(link, "/")
		popularRankingIds = append(popularRankingIds, slugify.Slugify(urlParts[len(urlParts)-1]))
	})
	popularRanking := Ranking{
		Source: "kissanime",
		Ranking: "popular",
		Series: popularRankingIds,
	}
	rankingsCollection.Upsert(bson.M{"source": "kissanime", "ranking": "popular"}, popularRanking)
	doc = GetDocument("http://kissanime.ru/AnimeList/NewAndHot")
	newestRankingIds := []string{}
	doc.Find("table.listing td:nth-child(1) a").Each(func(i int, s *goquery.Selection) {
		link, _ := s.Attr("href")
		urlParts := strings.Split(link, "/")
		newestRankingIds = append(newestRankingIds, slugify.Slugify(urlParts[len(urlParts)-1]))
	})
	newestRanking := Ranking{
		Source: "kissanime",
		Ranking: "newest",
		Series: newestRankingIds,
	}
	rankingsCollection.Upsert(bson.M{"source": "kissanime", "ranking": "newest"}, newestRanking)
}

func ScrapeKAGenres() {
	genres := []string{}
	doc := GetDocument("http://kissanime.ru/AnimeList")
	doc.Find("div.rightBox:contains(genres) div.barContent a").Each(func(i int, s *goquery.Selection) {
		genres = append(genres, strings.ToLower(strings.TrimSpace(s.Text())))
	})
	extrasCollection.Upsert(bson.M{"id": "kissanime", "key": "genres"}, bson.M{"$set": bson.M{"data": genres}})
}