package main

import (
	"fmt"
	"io"
	"regexp"

	"github.com/PuerkitoBio/goquery"
)

type AnimeType string

const (
	Series  AnimeType = "Animeserie"
	Special AnimeType = "Special"
	Movie   AnimeType = "Movie"
)

type Anime struct {
	EpisodesWatched uint16
	EpisodeCount    uint16
	Title           string
	Type            AnimeType
}

type Watchlist struct {
	Watched           []Anime
	CurrentlyWatching []Anime
	ToWatch           []Anime
	StoppedWatching   []Anime
}

func Parse(reader io.Reader) (Watchlist, error) {
	watchlist := Watchlist{}
	document, parseError := goquery.NewDocumentFromReader(reader)
	if parseError != nil {
		return watchlist, parseError
	}

	watchlist.Watched = parseTable(document.Find("a[name=state0]").Next())
	watchlist.CurrentlyWatching = parseTable(document.Find("a[name=state1]").Next())
	watchlist.ToWatch = parseTable(document.Find("a[name=state2]").Next())
	watchlist.StoppedWatching = parseTable(document.Find("a[name=state3]").Next())

	return watchlist, nil
}

func parseTable(table *goquery.Selection) []Anime {
	spaceCleaner := regexp.MustCompile(`\s{2,}`)
	rows := table.Children().Children()
	animes := make([]Anime, 0, rows.Size()-2)
	rows.Each(func(i int, s *goquery.Selection) {
		if i >= 2 {
			cells := s.Children()
			//First is just the status image, so we skip it.
			cell := cells.First().Next()

			anime := Anime{}

			//Name
			anime.Title = spaceCleaner.ReplaceAllString(cell.Find("a").Get(0).FirstChild.Data, " ")

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

			animes = append(animes, anime)
		}
	})

	return animes
}
