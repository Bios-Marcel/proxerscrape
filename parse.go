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

type Anime struct {
	// Data present in profile

	EpisodesWatched uint16
	EpisodeCount    uint16
	Title           string
	Type            AnimeType
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
	// Tags            []string
	// SpoilerTags     []string
	// UnconfirmedTags []string
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
				cell = cell.Next()
				switch key {
				case "Englischer Titel":
					{
						anime.EnglishTitle = cell.Get(0).FirstChild.Data
					}
				case "Deutscher Titel":
					{
						anime.GermanTitle = cell.Get(0).FirstChild.Data
					}
				case "Japanischer Titel":
					{
						anime.JapaneseTitle = cell.Get(0).FirstChild.Data
					}
				case "Synonym":
					{
						anime.Synonyms = append(anime.Synonyms, cell.Get(0).FirstChild.Data)
					}
				case "Genres":
					{
						for _, genreNode := range cell.Find("a[class=genreTag]").Nodes {
							anime.Generes = append(anime.Generes, genreNode.FirstChild.Data)
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

							anime.ReleasePeriod.FromSeason = season
							anime.ReleasePeriod.FromYear = year
							if len(children) >= 2 {
								season, year, err := parseSeason(children[1].FirstChild.Data)
								if err != nil {
									//TODO Handle properly; Can't return outer function or use error channel here.
									//FIXME Make custom loop, see impl of Each(...).
									return
								}

								anime.ReleasePeriod.ToSeason = season
								anime.ReleasePeriod.ToYear = year
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

// ParseProfileTabAnime takes an HTML dump of `Anime` of a profile and parses
// the contained watchlists. Note that the resulting Watchlist only contains
// certain data. You'll have to call WatchlistCategory.LoadExtraData on the
// respective lists if you require additional data.
func ParseProfileTabAnime(reader io.Reader) (Watchlist, error) {
	watchlist := Watchlist{}
	document, parseError := goquery.NewDocumentFromReader(reader)
	if parseError != nil {
		return watchlist, parseError
	}

	watchlist.Watched = WatchlistCategory{Data: parseProfileTabAnimeTable(document.Find("a[name=state0]").Next())}
	watchlist.CurrentlyWatching = WatchlistCategory{Data: parseProfileTabAnimeTable(document.Find("a[name=state1]").Next())}
	watchlist.ToWatch = WatchlistCategory{Data: parseProfileTabAnimeTable(document.Find("a[name=state2]").Next())}
	watchlist.StoppedWatching = WatchlistCategory{Data: parseProfileTabAnimeTable(document.Find("a[name=state3]").Next())}

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

func parseProfileTabAnimeTable(table *goquery.Selection) []*Anime {
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
