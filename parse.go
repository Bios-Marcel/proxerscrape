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

type MediaType string

const (
	Series  MediaType = "Animeserie"
	Special MediaType = "Special"
	Movie   MediaType = "Movie"

	Manga     MediaType = "Mangaserie"
	Webtoon   MediaType = "Webtoon"
	Manhwa    MediaType = "Manhwa"
	Doujinshi MediaType = "Doujinshi"
)

type Status string

const (
	// Finished means all episodes of have been released. This doesn't imply
	// that all seasons have the same status.
	Finished Status = "Abgeschlossen"
	// PreAiring means yet to be released.
	PreAiring Status = "Nicht erschienen (Pre-Airing)"
	// Airing means the series has been released, but not all episodes have
	// been released yet.
	Airing Status = "Airing"
)

// Season represents the four seasons of the year. Proxer.me represents these
// as integers internally.
type Season string

const (
	// Winter
	Q1 Season = "Q1"
	// Spring
	Q2 Season = "Q2"
	// Summer
	Q3 Season = "Q3"
	// Autumn
	Q4 Season = "Q4"
)

type ReleasePeriod struct {
	FromSeason Season
	FromYear   uint

	ToSeason Season
	ToYear   uint
}

// Media is the base for different types of media, such as Media or Manga.
// Note that names such as `EpisodesWatched` are anime specific, but work for
// Manga chapters as well. While use of interface would make the API more
// clean, I simply don't care ;).
type Media struct {
	// Data present in profile

	EpisodesWatched uint16
	EpisodeCount    uint16
	Title           string
	Type            MediaType
	ProxerURL       string
	Status          Status

	// Lazy data

	EnglishTitle  string
	GermanTitle   string
	JapaneseTitle string
	Synonyms      []string
	Rating        float64
	ReleasePeriod ReleasePeriod
	Generes       []string

	// Tags can't be parsed, since they aren't displayed on initial pageload.
	// FIXME A potential rework would be the use of:
	// https://pkg.go.dev/github.com/chromedp/chromedp
	// Tags            []string
	// SpoilerTags     []string
	// UnconfirmedTags []string
}

type WatchlistCategory struct {
	Data []*Media
	// extraDataLoaded tells whether the list already contains additional data
	// such as ratings, generes and other things not present on the profile
	// page. This flag prevents loading this data multiple times, since it is
	// constant data.
	extraDataLoaded bool
}

func (wc *WatchlistCategory) populateMediaWithExtraData(retrieveRawData MediaRawDataRetriever, item *Media) error {
	reader, cacheInvalidator, err := retrieveRawData(item)
	if err != nil {
		return err
	}
	//Make sure reader is being closed, even on panic or early return.
	defer reader.Close()

	document, errParse := goquery.NewDocumentFromReader(reader)
	if errParse != nil {
		return errParse
	}
	//Already close reader here, since we don't need it anymore either way.
	reader.Close()

	// Proxer keeps list entries even if the linked entry doesn't exist
	// anymore. Even picture and name still being presented isn't an
	// indicator.
	// FIXME If this happens again, I should check whether the "state"
	// field is relevant.
	title := document.Find("title").First().Get(0).FirstChild.Data
	if strings.Contains(title, "404") {
		log.Printf("Entry for '%s'(%s) is a dead link.\n", item.Title, item.ProxerURL)
		// Since we don't want to cache a 404 page, we need to invoke
		// the invalidator.
		if errInvalidate := cacheInvalidator(); errInvalidate != nil {
			log.Printf("Error invalidating cache entry for '%s': %s.\n", item.Title, errInvalidate)
		}
		// We don't want to error here, as we want to proceed parsing the
		// other entries, since there hasn't been an actual error here.
		return nil
	}

	//FIXME Provide way to login.
	potentialPleaseLoginTitle := document.Find("h3").First()
	if potentialPleaseLoginTitle.Length() == 1 &&
		strings.HasPrefix(
			strings.TrimSpace(potentialPleaseLoginTitle.Get(0).FirstChild.Data),
			"Bitte logge dich ein",
		) {
		log.Printf("Entry for '%s'(%s) requries a login, since the rating is most likeky 18+.\n", item.Title, item.ProxerURL)
		log.Println("If you wish to be able to retrieve these entries, please set the environment variables `LOGIN_COOKIE_KEY` and `LOGIN_COOKIE_VALUE` to `joomla_remember_me_XXX=XXX`.")
		// Since we don't want to cache a "please login ..." page, we need
		// to invoke the invalidator.
		if errInvalidate := cacheInvalidator(); errInvalidate != nil {
			log.Printf("Error invalidating cache entry for '%s': %s.\n", item.Title, errInvalidate)
		}
		// We don't want to error here, as we want to proceed parsing the
		// other entries, since there hasn't been an actual error here.
		return nil
	}

	// Ratelimited, this is a coding error.
	if document.Find("script[src='//www.google.com/recaptcha/api.js']").Length() > 0 {
		return errors.New("proxer.me ratelimit has been hit, captcha required")
	}

	document.Find("table[class=details]").First().Find("tbody > tr").Each(func(i int, s *goquery.Selection) {
		cell := s.Find("td").First()
		key := cell.Find("b").First().Get(0).FirstChild.Data
		cell = cell.Next()
		switch key {
		case "Englischer Titel":
			{
				item.EnglishTitle = cell.Get(0).FirstChild.Data
			}
		case "Deutscher Titel":
			{
				item.GermanTitle = cell.Get(0).FirstChild.Data
			}
		case "Japanischer Titel":
			{
				item.JapaneseTitle = cell.Get(0).FirstChild.Data
			}
		case "Synonym":
			{
				item.Synonyms = append(item.Synonyms, cell.Get(0).FirstChild.Data)
			}
		case "Genres":
			{
				for _, genreNode := range cell.Find("a[class=genreTag]").Nodes {
					item.Generes = append(item.Generes, genreNode.FirstChild.Data)
				}
			}
		case "Season":
			{
				children := cell.Find("a").Nodes
				if len(children) >= 1 {
					season, year, err := parseSeason(children[0].FirstChild.Data)
					if err != nil {
						//TODO Handle properly; Can't return outer function or use error channel here.
						//FIXME Make custom loop, see impl of Each(...).
						return
					}

					item.ReleasePeriod.FromSeason = season
					item.ReleasePeriod.FromYear = year
					if len(children) >= 2 {
						season, year, err := parseSeason(children[1].FirstChild.Data)
						if err != nil {
							//TODO Handle properly; Can't return outer function or use error channel here.
							//FIXME Make custom loop, see impl of Each(...).
							return
						}

						item.ReleasePeriod.ToSeason = season
						item.ReleasePeriod.ToYear = year
					}
				}
			}
		}
	})

	//Rating
	avgMatches := document.Find(".average").First()
	ratingString := avgMatches.Get(0).FirstChild.Data
	ratingFloat, errParse := strconv.ParseFloat(ratingString, 64)
	if errParse != nil {
		return errParse
	}

	item.Rating = ratingFloat
	return nil
}

// LoadExtraData will retrieve additional information for all animes in this
// category and load it into the respective *Anime. Calling this a second time
// will not have an effect.
func (wc *WatchlistCategory) LoadExtraData(retrieveRawData MediaRawDataRetriever) error {
	if wc.extraDataLoaded {
		return nil
	}

	var waitGroup sync.WaitGroup
	errChannel := make(chan error, 1)
	doneChannel := make(chan struct{}, 1)
	go func() {
		waitGroup.Wait()
		doneChannel <- struct{}{}
	}()

	// This loop only returns an error if we run into an error that's not
	//related to data, but something that's most likely a coding
	//error / feature not implemented.
	for _, item := range wc.Data {
		waitGroup.Add(1)
		go func(item *Media) {
			defer waitGroup.Done()
			if err := wc.populateMediaWithExtraData(retrieveRawData, item); err != nil {
				//FIXME The early exit here will cause the background routine
				//to run forever, since the waitGroup isn't done.
				errChannel <- err
			}
		}(item)
	}

	select {
	case err := <-errChannel:
		return err
	case <-doneChannel:
		wc.extraDataLoaded = true
		return nil
	}
}

func parseSeason(seasonRaw string) (Season, uint, error) {
	var year uint
	var seasonString string
	if _, err := fmt.Sscanf(seasonRaw, "%s %d", &seasonString, &year); err != nil {
		return "", 0, nil
	}
	var season Season
	switch seasonString {
	case "Winter":
		season = Q1
	case "Frühling":
		season = Q2
	case "Sommer":
		season = Q3
	case "Herbst":
		season = Q4
	}
	return season, year, nil
}

// Watchlist holds the different types of watchlists for a profile.
type Watchlist struct {
	Watched           WatchlistCategory
	CurrentlyWatching WatchlistCategory
	ToWatch           WatchlistCategory
	StoppedWatching   WatchlistCategory
}

// ParseProfileMediaTab takes an HTML dump any type of `Media` tab, such as
// `Anime` of a profile and parses the contained watchlists. Note that the
// resulting Watchlist only contains  certaindata. You'll have to call
// WatchlistCategory.LoadExtraData on the respective lists if you require
// additional data.
func ParseProfileMediaTab(reader io.Reader) (Watchlist, error) {
	watchlist := Watchlist{}
	document, parseError := goquery.NewDocumentFromReader(reader)
	if parseError != nil {
		return watchlist, parseError
	}

	watchlist.Watched = WatchlistCategory{Data: parseProfileTabMediaTable(document.Find("a[name=state0]").Next())}
	watchlist.CurrentlyWatching = WatchlistCategory{Data: parseProfileTabMediaTable(document.Find("a[name=state1]").Next())}
	watchlist.ToWatch = WatchlistCategory{Data: parseProfileTabMediaTable(document.Find("a[name=state2]").Next())}
	watchlist.StoppedWatching = WatchlistCategory{Data: parseProfileTabMediaTable(document.Find("a[name=state3]").Next())}

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

func parseProfileTabMediaTable(table *goquery.Selection) []*Media {
	spaceCleaner := regexp.MustCompile(`\s{2,}`)
	rows := table.Children().Children()
	entries := make([]*Media, 0, rows.Size()-2)
	rows.Each(func(i int, s *goquery.Selection) {
		if i >= 2 {
			item := Media{}

			cells := s.Children()
			cell := cells.First()

			//Status
			status, present := cell.Find("img").First().Attr("title")
			if !present {
				log.Panicf("Anime '%s' doesn't have a status.", item.Title)
			}
			item.Status = Status(status)

			//URL to info page
			cell = cell.Next()
			link := cell.Find("a").Get(0)
			item.ProxerURL = getAttribute(link, "href")

			//Name
			item.Title = spaceCleaner.ReplaceAllString(link.FirstChild.Data, " ")

			//Type of Media
			cell = cell.Next()
			baseType := cell.Get(0).FirstChild.Data
			// We don't wanna use the concrete types for anime, since they
			// don't provide value. This is different for manga, since there's
			// Manhwa, Webtoon and more.
			if baseType == string(Series) || baseType == string(Movie) || baseType == string(Special) {
				item.Type = MediaType(baseType)
			} else {
				if cell.Get(0).FirstChild.NextSibling != nil && cell.Get(0).FirstChild.NextSibling.NextSibling != nil {
					concreteType := cell.Get(0).FirstChild.NextSibling.NextSibling.Data
					item.Type = MediaType(concreteType)
				} else {
					item.Type = MediaType(baseType)
				}
			}
			fmt.Println(item.Type)

			//Skip review
			cell = cell.Next()

			//Episodecounts
			cell = cell.Next()
			_, scanError := fmt.Sscanf(cell.Find("span").Get(0).FirstChild.Data, "%d / %d", &item.EpisodesWatched, &item.EpisodeCount)
			if scanError != nil {
				panic(scanError)
			}

			entries = append(entries, &item)
		}
	})

	return entries
}
