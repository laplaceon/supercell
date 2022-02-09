package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jinzhu/gorm"
	_ "github.com/go-sql-driver/mysql"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"sync"
	"time"
)

// Logs are recorded to report status of certain services within the architecture
// The type refers to which service the log refers to: 1: Tester, 2: Crawler
// The status denotes the overall status of the message: 0: Unspecified, 1: Success, 2: Failure
// The message gives more details about the log entry
type Log struct {
	Type int
	Status int
	Message string
}

// Sources are the different sites that the crawler accesses
// Ready tells if the source can be scraped. Not ready sources are ignored
// Enabled also determines if source can be scraped, but determines if source is usable and is manually edited
type Source struct {
	Id string
	Name string
	Url string
	Ready bool
	Enabled bool
}

type Series struct {
	Source string
	Id string
	Url string
	Image string
	Title string
	AlternateTitles []string `bson:"alternate_titles" json:"alternate_titles"`
	Summary string
	Genres []string
	Completed bool
}

type Episode struct {
	Id string
	Link string
	Image string
	Name string
}

type Episodes struct {
	Source string
	Id string
	Listings []Listing
}

type Listing struct {
	Name string
	Episodes []Episode
}

type Ranking struct {
	Source string
	Ranking string
	Series []string
}

type Keys struct {
	UserAgent string `json:"user_agent"`
	BypassData []UrlCookies `json:"bypass_data"`
}

type UrlCookies struct {
	Url string `json:"url"`
	Ipv6 bool `json:"ipv6"`
	Cfclearance string `json:"cf_clearance"`
	Cfduid string `json:"cfduid"`
}

type Flag struct {
	SourceId string `json:"source_id"`
	Ready bool `json:"ready"`
}

var (
	hbQueue = make(chan ScrapeJob, 20000)
	lbQueue = make(chan ScrapeJob, 20000)
	lbJobs map[string][]ScrapeJob
	lbLock *sync.Mutex
	scraperQueue chan chan ScrapeJob
	client *http.Client
	userAgent string
	sources = make(map[string]Source)
	flags map[string]bool
	db *gorm.DB
	sourcesCollection *mgo.Collection
	seriesCollection *mgo.Collection
	episodesCollection *mgo.Collection
	rankingsCollection *mgo.Collection
	extrasCollection *mgo.Collection
	keysFileLocation string
	wg sync.WaitGroup
)

func ScrapeSources() {
	wg.Add(4)
	go StartKA()
	go StartMA()
	go StartGG()
	go Start9A()
}

// Collection of functions to prepare for scraping
func PreScrapeSources() {
	CloudflareBypass()
}

// Some sources have cloudflare IUAM always enabled. This allows the client to bypass such restrictions
func CloudflareBypass() {
	jar, _ := cookiejar.New(nil)
	var sourceKeys Keys

	keysFile, keysErr := os.Open(keysFileLocation)
	if keysErr != nil {
		fmt.Println("Error opening keys file")
		return
	}

	decodeErr := json.NewDecoder(keysFile).Decode(&sourceKeys)
	if decodeErr != nil {
		fmt.Println("Error decoding keys file")
		return
	}

	urlsAdded := make(map[string]bool)

	sourceKeyItems := sourceKeys.BypassData

	for i := 0; i < len(sourceKeyItems); i++ {
		sourceKeyItem := sourceKeyItems[i]
		if !urlsAdded[sourceKeyItem.Url] {
			cookies := []*http.Cookie{}
			cookies = append(cookies, &http.Cookie{Name: "cf_clearance", Value: sourceKeyItem.Cfclearance})
			cookies = append(cookies, &http.Cookie{Name: "__cfduid", Value: sourceKeyItem.Cfduid})
			baseUrl, _ := url.Parse("https://" + sourceKeyItem.Url)

			jar.SetCookies(baseUrl, cookies)

			urlsAdded[sourceKeyItem.Url] = true
			fmt.Println("Processed", sourceKeyItem.Url)
		}

	}

	userAgent = sourceKeys.UserAgent

	client.Jar = jar
}

// Listeners for rate limited sources, they wrap the scrape function for their respective source
// This allows scrape restarts for individual links
func StartLimitedListeners() {
	lbJobs = make(map[string][]ScrapeJob)
	go func() {
		for {
			ListenScrapeMA()
		}
	}()
	go func() {
		for {
			ListenScrape9A()
		}
	}()
}

// Only runs when in worker mode
func InitScrapers(numScrapers int) {
	scraperQueue = make(chan chan ScrapeJob, numScrapers)

	// Create as many scrapers as specified for single machine concurrent scraping
	for i := 0; i < numScrapers; i++ {
		scraper := InitScraper(scraperQueue)
		scraper.Start()
	}

	StartLimitedListeners()

	lbLock = &sync.Mutex{}

	// Start a distributor that waits for links from a queue and assigns it to available scrapers
	go func() {
		for {
			select {
			case job := <-hbQueue:
				go func() {
					scraper := <-scraperQueue
					scraper <- job
				}()
			}
		}
	}()

	// Can probably combine both distributors
	// Needs more research
	go func() {
		for {
			select {
			case job := <-lbQueue:
				switch job.Source {
				case "masteranime":
					if flags["masteranime"] {
						lbLock.Lock()
						lbJobs["masteranime"] = append(lbJobs["masteranime"], job)
						lbLock.Unlock()
					}
				case "9anime":
					if flags["9anime"] {
						lbLock.Lock()
						lbJobs["9anime"] = append(lbJobs["9anime"], job)
						lbLock.Unlock()
					}
				}
			}
		}
	}()
}

func main() {
	var flagsFileLocation string
	mode := flag.String("mode", "update", "scrape mode")
	flag.StringVar(&supervisorIp, "supervisor", "", "the ip address or hostname of the supervisor node")
	flag.StringVar(&sinkIp, "sink", "", "the ip address or hostname of the sink node")
	flag.StringVar(&keysFileLocation, "keys", "", "file containing cf key data")
	flag.StringVar(&flagsFileLocation, "flags", "", "file containing flag data")
	numScrapers := flag.Int("scrapers", 2, "how many scrapers to employ for the current worker")
	mysqlUser := flag.String("mysqluser", "", "the username for the mysql database")
	mysqlPass := flag.String("mysqlpass", "", "the password for the mysql database")
	mongoUser := flag.String("mongouser", "", "the username for the mongo database")
	mongoPass := flag.String("mongopass", "", "the password for the mongo database")
	flag.Parse()

	// No supervisor node locations implies this is the supervisor node
	leader := supervisorIp == ""

	if leader {
		var cErr error
		db, cErr = gorm.Open("mysql", *mysqlUser + ":" + *mysqlPass + "@tcp(" + sinkIp + ":3306)/typhoon?charset=utf8&parseTime=True&loc=Local")
		defer db.Close()
		if cErr != nil {
			fmt.Println("Failed to connect to database")
			return
		}
	}

	sess, err := mgo.Dial("mongodb://" + *mongoUser + ":" + *mongoPass + "@" + sinkIp + ":27017")
	defer sess.Close()
	if(err != nil) {
		fmt.Println("Failed to connect to document database")
		return
	}

	flags = make(map[string]bool)

	flagsA := []Flag{}
	flagsFile, flagsErr := os.Open(flagsFileLocation)
	if flagsErr != nil {
		fmt.Println("Error opening flags file")
		return
	}

	decodeErr := json.NewDecoder(flagsFile).Decode(&flagsA)
	if decodeErr != nil {
		fmt.Println("Error decoding flags file")
		return
	}

	for i := 0; i < len(flagsA); i++ {
		flags[flagsA[i].SourceId] = flagsA[i].Ready
	}

	docDB := sess.DB("typhoon")
	sourcesCollection = docDB.C("sources")
	seriesCollection = docDB.C("series")
	episodesCollection = docDB.C("episodes")
	rankingsCollection = docDB.C("rankings")
	extrasCollection = docDB.C("source_extras")

	if leader {
		InitiateSupervisor()

		fmt.Println("Waiting for workers to join...")

		// Wait for workers to join in a certain time window
		quitChan := make(chan bool)
		go WaitForWorkers(quitChan)

		// Send kill signal to stop waiting for new workers
		// Due to limitations in pipeline as a load balancing method, as well as scrape tasks not adding an
		// idle state to workers, it is difficult to handle newly incoming workers to get equal number of links as its peers
		// Therefore, we must set a fixed joining period and only work with those workers
		time.Sleep(5 * time.Second)
		quitChan <- true
		close(quitChan)

		// We need at least 1 worker to begin scraping since the supervisor only distributes tasks
		if numWorkers == 0 {
			fmt.Println("No workers joined :(")
			return
		}

		fmt.Println("I'm going to wait a little bit more")

		time.Sleep(1 * time.Second)

		fmt.Println("Alright, it's time to start work!")

		switch *mode {
			case "rebuild":
				fmt.Println("Rebuild mode")
				seriesCollection.DropCollection()
				episodesCollection.DropCollection()
				rankingsCollection.DropCollection()
				extrasCollection.RemoveAll(bson.M{"key": bson.M{"$in": []string{"rankings", "genres"}}})
			default:
				fmt.Println("Update mode")
		}

		// Load sources from document database and put it into a map for easy access
		results := []Source{}
		accessErr := sourcesCollection.Find(nil).All(&results)
		if accessErr != nil {
			panic(accessErr)
			fmt.Println("Failed to access document database sources.")
			return
		}

		for i := 0; i < len(results); i++ {
			result := results[i]
			sources[result.Id] = result
		}
	} else {
		InitiateWorker()
		tracker.Send("started", 0)

		// Wait for a response from the supervisor, if one is not received, the worker exits
		statusChan := make(chan bool)
		go func(statusChan chan bool) {
			tracker.Recv(0)
			statusChan <- true
		}(statusChan)

		select {
			case <-statusChan:
				fmt.Println("Received ok from supervisor!")
			case <-time.After(5 * time.Second):
				fmt.Println("Supervisor taking too long to respond. Exiting...")
				return
		}
		close(statusChan)

		InitScrapers(*numScrapers)
	}

	client = &http.Client{}

	PreScrapeSources()

	defer tracker.Close()
	defer notifier.Close()

	if leader {
		defer supervisor.Close()

		now := time.Now()

		ScrapeSources()

		wg.Wait()

		// Send completion signal to workers
		notifier.Send("notifier done", 0)

		// Blocking wait for workers to send their completion signals
		// If signals are sent before, they will be buffered, as per ZMQ
		fmt.Println("Waiting")
		WaitForWorkersToFinish()

		diff := time.Since(now)

		fmt.Println("Done!")

		// Log completion
		completionLog := Log{Type: 2, Status: 1, Message: "Completed scrape in " + diff.String()}
		db.Create(&completionLog)
	} else {
		defer worker.Close()

		go WaitForWork()

		wg.Add(1)

		go ListenForSupervisorFinish()

		wg.Wait()

		fmt.Println("Done!")

		// Report complete
		tracker.Send("finished", 0)

		// Wait for response before killing to avoid closing socket connection
		tracker.Recv(0)
	}
}
