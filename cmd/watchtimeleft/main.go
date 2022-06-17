package main

import (
	"fmt"
	"os"
	"time"

	parse "github.com/Bios-Marcel/proxerscrape"
)

func main() {
	watchlist, parseError := parse.ParseProfileTabAnime(os.Stdin)
	if parseError != nil {
		panic(parseError)
	}

	var currentlyWatchingLeft time.Duration
	fmt.Printf("Currently Watching (%d)\n", len(watchlist.CurrentlyWatching.Data))
	for _, item := range watchlist.CurrentlyWatching.Data {
		currentlyWatchingLeft += getWatchtimeLeft(item)
		fmt.Println(item.Title)
	}

	fmt.Printf("\nTo Watch (%d)\n", len(watchlist.ToWatch.Data))
	var toWatchLeft time.Duration
	for _, item := range watchlist.ToWatch.Data {
		toWatchLeft += getWatchtimeLeft(item)
		fmt.Println(item.Title)
	}

	fmt.Printf("\n%s hours (%d anime) on to watch list.\n", toWatchLeft, len(watchlist.ToWatch.Data))
	fmt.Printf("%s hours (%d anime) on currently watching list.\n", currentlyWatchingLeft, len(watchlist.CurrentlyWatching.Data))
}

func getWatchtimeLeft(anime *parse.Anime) time.Duration {
	switch anime.Type {
	case parse.Series:
		return time.Duration(time.Duration(anime.EpisodeCount-anime.EpisodesWatched) * 20 * time.Minute)
	case parse.Movie:
		return time.Duration(90 * time.Minute)
	case parse.Special:
		//What to put here?
		return time.Duration(time.Duration(anime.EpisodeCount-anime.EpisodesWatched) * 7 * time.Minute)
	}

	return 0
}
