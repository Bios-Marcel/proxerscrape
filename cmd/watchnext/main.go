package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/Bios-Marcel/proxerscrape"
)

func main() {
	watchlist, parseError := proxerscrape.Parse(os.Stdin)
	if parseError != nil {
		panic(parseError)
	}

	cache := proxerscrape.Cache{
		Query: proxerscrape.QueryRateLimited,
	}
	if err := watchlist.ToWatch.LoadExtraData(cache.RetrieveAnimeRawData); err != nil {
		panic(err)
	}

	orderedByReview := make([]*proxerscrape.Anime, len(watchlist.ToWatch.Data))
	copy(orderedByReview, watchlist.ToWatch.Data)

	//First we filter pre-airing ones, since we can't watch them anyway.
	for i := 0; i < len(orderedByReview); i++ {
		anime := orderedByReview[i]
		if anime.Status == proxerscrape.PreAiring {
			if i == len(orderedByReview)-1 {
				orderedByReview = orderedByReview[:i]
			} else {
				orderedByReview[i] = orderedByReview[len(orderedByReview)-1]
				orderedByReview = orderedByReview[:len(orderedByReview)-1]
			}
			i--
		}
	}

	if len(orderedByReview) > 0 {
		//Now we sort, so we can take the highest rated one.
		sort.Slice(orderedByReview, func(a, b int) bool {
			return orderedByReview[a].Rating > orderedByReview[b].Rating
		})
		fmt.Println("Next, you should watch:", orderedByReview[0].Title)
	} else {
		fmt.Println("It seems like there's nothing available on your watchlist right now.")
	}
}
