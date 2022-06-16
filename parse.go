package proxerscrape

import (
	"errors"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

type AnimeType string

const (
	Series  AnimeType = "Animeserie"
	Special AnimeType = "Special"
	Movie   AnimeType = "Movie"
)

type Status string

const (
	// Finished means all episodes of have been released. This doesn't imply
	// that all seasons have the same status.
	Finished Status = "Abgeschlossen"
	// PreAiring means yet to be released.
	PreAiring = "Nicht erschienen (Pre-Airing)"
	// Airing means the series has been released, but not all episodes have
	// been released yet.
	Airing = "Airing"
)

type Anime struct {
	// Data present in profile

	EpisodesWatched uint16
	EpisodeCount    uint16
	Title           string
	Type            AnimeType
	ProxerURL       string
	Status          Status

	// Lazy data

	Rating  float64
	Generes []string
}

type WatchlistCategory struct {
	Data []*Anime
	// extraDataLoaded tells whether the list already contains additional data
	// such as ratings, generes and other things not present on the profile
	// page. This flag prevents loading this data multiple times, since it is
	// constant data.
	extraDataLoaded bool
}

// LoadExtraData will retrieve additional information for all animes in this
// category and load it into the respective *Anime. Calling this a second time
// will not have an effect.
func (wc *WatchlistCategory) LoadExtraData(retrieveRawData func(*Anime) (io.ReadCloser, CacheInvalidator, error)) error {
	if wc.extraDataLoaded {
		return nil
	}

	var waitGroup sync.WaitGroup
	errChannel := make(chan error)
	doneChannel := make(chan struct{})
	go func() {
		waitGroup.Wait()
		doneChannel <- struct{}{}
	}()

	// This loop only returns an error if we run into an error that's not
	//related to data, but something that's most likely a coding
	//error / feature not implemented.
	for _, anime := range wc.Data {
		waitGroup.Add(1)
		go func(anime *Anime) {
			defer waitGroup.Done()
			reader, cacheInvalidator, err := retrieveRawData(anime)
			if err != nil {
				errChannel <- err
				return
			}
			defer reader.Close()

			document, errParse := goquery.NewDocumentFromReader(reader)
			if errParse != nil {
				errChannel <- errParse
				return
			}

			// Proxer keeps list entries even if the linked entry doesn't exist
			// anymore. Even picture and name still being presented isn't an
			// indicator.
			// FIXME If this happens again, I should check whether the "state"
			// field is relevant.
			title := document.Find("title").First().Get(0).FirstChild.Data
			if strings.Contains(title, "404") {
				log.Printf("Entry for '%s'(%s) is a dead link.\n", anime.Title, anime.ProxerURL)
				// Since we don't want to cache a 404 page, we need to invoke
				// the invalidator.
				if errInvalidate := cacheInvalidator(); errInvalidate != nil {
					log.Printf("Error invalidating cache entry for '%s': %s.\n", anime.Title, errInvalidate)
				}
				return
			}

			//FIXME Provide way to login.
			potentialPleaseLoginTitle := document.Find("h3").First()
			if potentialPleaseLoginTitle.Length() == 1 &&
				strings.HasPrefix(
					strings.TrimSpace(potentialPleaseLoginTitle.Get(0).FirstChild.Data),
					"Bitte logge dich ein",
				) {
				log.Printf("Entry for '%s'(%s) requries a login, since the rating is most likeky 18+.\n", anime.Title, anime.ProxerURL)
				log.Println("If you wish to be able to retrieve these entries, please set the environment variables `LOGIN_COOKIE_KEY` and `LOGIN_COOKIE_VALUE` to `joomla_remember_me_XXX=XXX`.")
				// Since we don't want to cache a "please login ..." page, we need
				// to invoke the invalidator.
				if errInvalidate := cacheInvalidator(); errInvalidate != nil {
					log.Printf("Error invalidating cache entry for '%s': %s.\n", anime.Title, errInvalidate)
				}
				return
			}

			// Ratelimited, this is a coding error.
			if document.Find("script[src='//www.google.com/recaptcha/api.js']").Length() > 0 {
				errChannel <- errors.New("proxer.me ratelimit has been hit, captcha required")
				return
			}

			document.Find("table[class=details]").First().Find("tbody > tr").Each(func(i int, s *goquery.Selection) {
				cell := s.Find("td").First()
				key := cell.Find("b").First().Get(0).FirstChild.Data
				switch key {
				case "Genres":
					{
						for _, genreNode := range cell.Next().Find("a[class=genreTag]").Nodes {
							anime.Generes = append(anime.Generes, genreNode.FirstChild.Data)
						}

						//For now we break, since we don't care about the other properties.
					}
				}
			})

			//Rating
			avgMatches := document.Find(".average").First()
			ratingString := avgMatches.Get(0).FirstChild.Data
			ratingFloat, errParse := strconv.ParseFloat(ratingString, 64)
			if errParse != nil {
				errChannel <- errParse
				return
			}

			anime.Rating = ratingFloat
		}(anime)
	}

	select {
	case err := <-errChannel:
		return err
	case <-doneChannel:
		wc.extraDataLoaded = true
		return nil
	}
}

type Watchlist struct {
	Watched           WatchlistCategory
	CurrentlyWatching WatchlistCategory
	ToWatch           WatchlistCategory
	StoppedWatching   WatchlistCategory
}

func Parse(reader io.Reader) (Watchlist, error) {
	watchlist := Watchlist{}
	document, parseError := goquery.NewDocumentFromReader(reader)
	if parseError != nil {
		return watchlist, parseError
	}

	watchlist.Watched = WatchlistCategory{Data: parseTable(document.Find("a[name=state0]").Next())}
	watchlist.CurrentlyWatching = WatchlistCategory{Data: parseTable(document.Find("a[name=state1]").Next())}
	watchlist.ToWatch = WatchlistCategory{Data: parseTable(document.Find("a[name=state2]").Next())}
	watchlist.StoppedWatching = WatchlistCategory{Data: parseTable(document.Find("a[name=state3]").Next())}

	return watchlist, nil
}

func getAttribute(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}

	return ""
}

func parseTable(table *goquery.Selection) []*Anime {
	spaceCleaner := regexp.MustCompile(`\s{2,}`)
	rows := table.Children().Children()
	animes := make([]*Anime, 0, rows.Size()-2)
	rows.Each(func(i int, s *goquery.Selection) {
		if i >= 2 {
			anime := Anime{}

			cells := s.Children()
			cell := cells.First()

			//Status
			status, present := cell.Find("img").First().Attr("title")
			if !present {
				log.Panicf("Anime '%s' doesn't have a status.", anime.Title)
			}
			anime.Status = Status(status)

			//URL to info page of anime
			cell = cell.Next()
			link := cell.Find("a").Get(0)
			anime.ProxerURL = getAttribute(link, "href")

			//Name
			anime.Title = spaceCleaner.ReplaceAllString(link.FirstChild.Data, " ")

			//Type of Anime
			cell = cell.Next()
			anime.Type = AnimeType(cell.Get(0).FirstChild.Data)

			//Skip review
			cell = cell.Next()

			//Episodecounts
			cell = cell.Next()
			_, scanError := fmt.Sscanf(cell.Find("span").Get(0).FirstChild.Data, "%d / %d", &anime.EpisodesWatched, &anime.EpisodeCount)
			if scanError != nil {
				panic(scanError)
			}

			animes = append(animes, &anime)
		}
	})

	return animes
}
