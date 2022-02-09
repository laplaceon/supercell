package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"gopkg.in/mgo.v2/bson"
	"strconv"
	"strings"
	"time"
)

func StartMA() {
	if sources["masteranime"].Ready && sources["masteranime"].Enabled {
		CycleMADirectory(1)
		ScrapeMARankings()
		ScrapeMAGenres()
	}
	wg.Done()
}

func CycleMADirectory(page int) {
	directoryUrl := "http://www.masterani.me/api/anime/filter?order=title&page=" + fmt.Sprintf("%v", page)
	fmt.Println("Scraping directory page: " + directoryUrl)
	doc := GetJson(directoryUrl)
	if doc == nil {
		fmt.Println("Fuck")
	}
	root := doc.(map[string]interface{})
	data := root["data"].([]interface{})
	for i := 0 ; i < len(data); i++ {
		series := data[i].(map[string]interface{})
		link := "http://www.masterani.me/api/anime/" + strconv.FormatFloat(series["id"].(float64), 'f', -1, 64) + "/detailed"
		SendWork("masteranime", link, true)
	}
	if root["current_page"] != root["last_page"] {
		page++
		CycleMADirectory(page)
	}
}

func ListenScrapeMA() {
	if len(lbJobs["masteranime"]) > 0 {
		lbLock.Lock()
		top := lbJobs["masteranime"][0]
		lbJobs["masteranime"] = lbJobs["masteranime"][1:]
		lbLock.Unlock()
		ScrapeMA(top.Link)
		wg.Done()
	}
}

func ScrapeMA(link string) {
	source := "masteranime"
	urlParts := strings.Split(link, "/")
	id := urlParts[len(urlParts)-2]
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
		doc := GetJson(link)
		if doc == nil {
			// Retry after some sleep time
			time.Sleep(60000 * time.Millisecond)
			ScrapeMA(link)
		} else {
			root := doc.(map[string]interface{})
			info := root["info"].(map[string]interface{})
			status := info["status"].(float64)
			if status == 2 {
				return
			}
			genres := []string{}
			genresList := root["genres"].([]interface{})
			for i := 0; i < len(genresList); i++ {
				genre := genresList[i].(map[string]interface{})
				genres = append(genres, strings.ToLower(genre["name"].(string)))
			}
			alternateTitles := []string{}
			synonyms := root["synonyms"].([]interface{})
			for i := 0; i < len(synonyms); i++ {
				synonym := synonyms[i].(map[string]interface{})
				alternateTitles = append(alternateTitles, synonym["title"].(string))
			}
			slug := info["slug"].(string)
			episodes := []Episode{}
			episodesList := root["episodes"].([]interface{})
			for i := 0; i < len(episodesList); i++ {
				episode := episodesList[i].(map[string]interface{})
				episodeInfo := episode["info"].(map[string]interface{})
				thumbnail := ""
				if episode["thumbnail"] != nil {
					thumbnail = "https://cdn.masterani.me/episodes/" + episode["thumbnail"].(string)
				}
				title := "EP. " + episodeInfo["episode"].(string)
				if episodeInfo["title"] != nil {
					title = episodeInfo["title"].(string)
				}
				episodeNum := episodeInfo["episode"].(string)
				episodes = append(episodes, Episode{Id: episodeNum, Name: title, Image: thumbnail, Link: "http://www.masterani.me/anime/watch/" + slug + "/" + episodeNum})
			}
			listing := Listing{
				Name: "",
				Episodes: episodes,
			}
			episodesData := Episodes{Source: source, Id: id, Listings: []Listing{listing}}
			series := Series{
				Source: source,
				Id: id,
				Url: "http://masterani.me/anime/info/" + info["slug"].(string),
				Title: info["title"].(string),
				AlternateTitles: alternateTitles,
				Image: "http://cdn.masterani.me/poster/1/" + root["poster"].(string),
				Summary: info["synopsis"].(string),
				Genres: genres,
				Completed: info["status"].(float64) == 0,
			}
			
			seriesCollection.Upsert(bson.M{"source": source, "id": id}, series)
			_, episodesErr := episodesCollection.Upsert(bson.M{"source": source, "id": id}, episodesData)
			
			if episodesErr != nil {
				db.Create(&Log{Type: 2, Status: 2, Message: "Failed to update episodes for " + source + " / " + id})
			}
			
			fmt.Println("Inserted source: " + source + " id: " + id)
		}
	} else {
		fmt.Println("Source: " + source + " id: " + id + " not added. Already complete")
	}
}

func ScrapeMARankings() {
	doc := GetJson("http://www.masterani.me/api/anime/filter?order=score_desc&page=1")
	popularRankingIds := []string{}
	root := doc.(map[string]interface{})
	series := root["data"].([]interface{})
	for i := 0; i < len(series); i++ {
		s := series[i].(map[string]interface{})
		popularRankingIds = append(popularRankingIds, strconv.FormatFloat(s["id"].(float64), 'f', -1, 64))
	}
	popularRanking := Ranking{
		Source: "masteranime",
		Ranking: "popular",
		Series: popularRankingIds,
	}
	rankingsCollection.Upsert(bson.M{"source": "masteranime", "ranking": "popular"}, popularRanking)
	doc = GetJson("http://www.masterani.me/api/anime/filter?order=score_desc&page=1&status=1")
	newestRankingIds := []string{}
	root = doc.(map[string]interface{})
	series = root["data"].([]interface{})
	for i := 0; i < len(series); i++ {
		s := series[i].(map[string]interface{})
		newestRankingIds = append(newestRankingIds, strconv.FormatFloat(s["id"].(float64), 'f', -1, 64))
	}
	newestRanking := Ranking{
		Source: "masteranime",
		Ranking: "newest",
		Series: newestRankingIds,
	}
	rankingsCollection.Upsert(bson.M{"source": "masteranime", "ranking": "newest"}, newestRanking)
}

func ScrapeMAGenres() {
	genres := []string{}
	doc := GetDocument("http://www.masterani.me/anime")
	doc.Find("div.genres-pop-out div.item").Each(func(i int, s *goquery.Selection) {
		genres = append(genres, strings.ToLower(strings.TrimSpace(s.Text())))
	})
	extrasCollection.Upsert(bson.M{"id": "masteranime", "key": "genres"}, bson.M{"$set": bson.M{"data": genres}})
}