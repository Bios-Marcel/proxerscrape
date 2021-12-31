package main

import (
	"fmt"
	"os"
	"time"
)

func main() {
	watchlist, parseError := Parse(os.Stdin)
	if parseError != nil {
		panic(parseError)
	}

	var currentlyWatchingLeft time.Duration
	fmt.Printf("Currently Watching (%d)\n", len(watchlist.CurrentlyWatching))
	for _, item := range watchlist.CurrentlyWatching {
		currentlyWatchingLeft += getWatchtimeLeft(item)
		fmt.Println(item.Title)
	}

	fmt.Printf("\nTo Watch (%d)\n", len(watchlist.ToWatch))
	var toWatchLeft time.Duration
	for _, item := range watchlist.ToWatch {
		toWatchLeft += getWatchtimeLeft(item)
		fmt.Println(item.Title)
	}

	fmt.Printf("\n%s hours on to watch list.\n", toWatchLeft)
	fmt.Printf("%s hours on currently watching list.\n", currentlyWatchingLeft)
}

func getWatchtimeLeft(anime Anime) time.Duration {
	switch anime.Type {
	case Series:
		return time.Duration(time.Duration(anime.EpisodeCount-anime.EpisodesWatched) * 20 * time.Minute)
	case Movie:
		return time.Duration(90 * time.Minute)
	case Special:
		//What to put here?
		return time.Duration(time.Duration(anime.EpisodeCount-anime.EpisodesWatched) * 7 * time.Minute)
	}

	return 0
}
