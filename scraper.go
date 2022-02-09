package main

type ScrapeJob struct {
	Source string
	Link string
	Limited bool
}

type Scraper struct {
	Job chan ScrapeJob
	ScraperQueue chan chan ScrapeJob
}

func InitScraper(scraperQueue chan chan ScrapeJob) Scraper {
	return Scraper {
		Job: make(chan ScrapeJob),
		ScraperQueue: scraperQueue,
	}
}

func (s *Scraper) Start() {
	go func() {
		for {
			s.ScraperQueue <- s.Job

			job := <-s.Job
			
			switch job.Source {
				case "gogoanime":
					if flags["gogoanime"] {
						ScrapeGG(job.Link)
					}
				case "kissanime":
					if flags["kissanime"] {
						ScrapeKA(job.Link)
					}
				case "animehaven":
					if flags["animehaven"] {
						ScrapeAH(job.Link)
					}
				case "9anime":
					if flags["9anime"] {
						Scrape9A(job.Link)
					}
				default:
					// Do nothing, drop job
			}
			wg.Done()
		}
	}()
}