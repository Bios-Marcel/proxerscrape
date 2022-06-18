package proxerscrape

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var (
	//The ratelimiters all try to slightly stay below official limits.
	//Limits: https://github.com/proxer/ProxerLibJava/blob/f55585b485c8eff6da944032690fe4de40a58e65/library/src/main/kotlin/me/proxer/library/internal/interceptor/RateLimitInterceptor.kt#L25

	// Original limit for `/info/xxx/anime` is 20r/6min.
	animeRateLimiter = NewLimiter(18, time.Minute*6)
	// Original limit for `/info/xxx/anime` is 10r/5min.
	mangaRateLImiter = NewLimiter(8, time.Minute*5)
	// Original limit for `/info/xxx/anime` is 40r/6min.
	userRateLImiter = NewLimiter(38, time.Minute*6)

	cacheBaseDir, profileTabCacheDir string
	loginCookieKey, loginCookieValue string
)

func init() {
	var err error
	cacheBaseDir, err = os.UserCacheDir()
	if err != nil {
		panic(err)
	}

	cacheBaseDir = filepath.Join(cacheBaseDir, "proxerscrape")
	profileTabCacheDir = filepath.Join(cacheBaseDir, "profile")
	if err = os.MkdirAll(profileTabCacheDir, os.ModePerm); err != nil {
		panic(err)
	}

	loginCookieKey = os.Getenv("LOGIN_COOKIE_KEY")
	loginCookieValue = os.Getenv("LOGIN_COOKIE_VALUE")
}

func getCacheIdentifier(anime *Media) string {
	return regexp.MustCompile(`/info/(\d+).*`).FindStringSubmatch(anime.ProxerURL)[1]
}

type Cache struct {
	QueryMedia                 func(*Media) (*http.Response, error)
	QueryProfileTab            func(string, ProfileTabType) (*http.Response, error)
	AnimeQueryRatelimiter      *Limiter
	MangaQueryRatelimiter      *Limiter
	ProfileTabQueryRatelimiter *Limiter
}

type MediaRawDataRetriever func(*Media) (io.ReadCloser, CacheInvalidator, error)

// CacheInvalidator is a simple interface to make sure the caller of
// RetrieveAnimeRawData know what the second parameter means. The invalidator
// is used for removing an item from cache. This can be used by a parser if
// it deems an item to be invalid.
type CacheInvalidator func() error

type ProfileTabType string

const (
	ProfileTabAnime ProfileTabType = "anime"
	ProfileTabManga ProfileTabType = "manga"
	ProfileTabNovel ProfileTabType = "novel"
)

func (cache *Cache) RetrieveProfileTabRawData(profileId string, tabType ProfileTabType) (io.ReadCloser, CacheInvalidator, error) {
	cacheFilePath := filepath.Join(profileTabCacheDir, string(tabType)+".html")
	return retrieve(cacheFilePath, tabType, func(tabType ProfileTabType) (*http.Response, error) {
		if cache.ProfileTabQueryRatelimiter != nil {
			cache.ProfileTabQueryRatelimiter.Wait()
		}
		return cache.QueryProfileTab(profileId, tabType)
	})
}

// RetrieveAnimeRawData retrieves the HTML page for a media entry, which could
// for example be an anime or a manga, allowing further processing to retrieve
// additional information. If any data has been found both a reader and an
// invalidator is returned. The invalidator can be used if whatever instance
// receiveing the data, deems that it is invalid an should be removed from
// cache.
func (cache *Cache) RetrieveAnimeRawData(item *Media) (io.ReadCloser, CacheInvalidator, error) {
	cacheIdentifier := getCacheIdentifier(item)
	cacheFilePath := filepath.Join(cacheBaseDir, cacheIdentifier+".html")
	return retrieve(cacheFilePath, item, func(item *Media) (*http.Response, error) {
		if cache.AnimeQueryRatelimiter != nil {
			cache.AnimeQueryRatelimiter.Wait()
		}
		return cache.QueryMedia(item)
	})
}

// RetrieveMangaRawData retrieves the HTML page for a media entry, which could
// for example be an anime or a manga, allowing further processing to retrieve
// additional information. If any data has been found both a reader and an
// invalidator is returned. The invalidator can be used if whatever instance
// receiveing the data, deems that it is invalid an should be removed from
// cache.
func (cache *Cache) RetrieveMangaRawData(item *Media) (io.ReadCloser, CacheInvalidator, error) {
	cacheIdentifier := getCacheIdentifier(item)
	cacheFilePath := filepath.Join(cacheBaseDir, cacheIdentifier+".html")
	return retrieve(cacheFilePath, item, func(item *Media) (*http.Response, error) {
		if cache.MangaQueryRatelimiter != nil {
			cache.MangaQueryRatelimiter.Wait()
		}
		return cache.QueryMedia(item)
	})
}

func retrieve[T any](cacheFilePath string, item T, query func(T) (*http.Response, error)) (io.ReadCloser, CacheInvalidator, error) {
	cacheInvalidator := func() error {
		return os.Remove(cacheFilePath)
	}
	file, err := os.Open(cacheFilePath)
	if err == nil {
		return file, cacheInvalidator, nil
	}

	if !os.IsNotExist(err) {
		return nil, nil, err
	}

	response, err := query(item)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()

	cacheFileWriter, err := os.Create(cacheFilePath)
	if err != nil {
		return nil, nil, err
	}
	defer cacheFileWriter.Close()

	//This doesn't actually copy the data underneath and therefore allows
	//us to read the same buffer twice without actually reallocating the
	//byte array. We need this since we also can't read the body twice.
	var buffer bytes.Buffer
	if _, err = io.Copy(&buffer, response.Body); err != nil {
		return nil, nil, err
	}
	bufferCopy := buffer

	if _, err = io.Copy(cacheFileWriter, &buffer); err != nil {
		return nil, nil, err
	}

	return io.NopCloser(&bufferCopy), cacheInvalidator, nil
}

func CreateDefaultCache() *Cache {
	return &Cache{
		QueryMedia: func(item *Media) (*http.Response, error) {
			return QueryDirectly("https://proxer.me" + item.ProxerURL)
		},
		QueryProfileTab: func(profileId string, tabType ProfileTabType) (*http.Response, error) {
			return QueryDirectly(fmt.Sprintf("https://proxer.me/user/%s/%s", profileId, tabType))
		},
		AnimeQueryRatelimiter:      animeRateLimiter,
		MangaQueryRatelimiter:      mangaRateLImiter,
		ProfileTabQueryRatelimiter: userRateLImiter,
	}
}
