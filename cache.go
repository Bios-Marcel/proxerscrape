package proxerscrape

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var (
	// Ratelimit is 20 requests per 6 minutes, we try to stay below this to
	// make sure stuff doesn't go awry.
	rateLimiter                                = NewLimiter(15, time.Minute*6)
	cacheDir, loginCookieKey, loginCookieValue string
)

func init() {
	var err error
	cacheDir, err = os.UserCacheDir()
	if err != nil {
		panic(err)
	}

	cacheDir = filepath.Join(cacheDir, "proxerscrape")
	if err = os.MkdirAll(cacheDir, os.ModePerm); err != nil {
		panic(err)
	}

	loginCookieKey = os.Getenv("LOGIN_COOKIE_KEY")
	loginCookieValue = os.Getenv("LOGIN_COOKIE_VALUE")
}

func getCacheIdentifier(anime *Anime) string {
	return regexp.MustCompile(`/info/(\d+).*`).FindStringSubmatch(anime.ProxerURL)[1]
}

// CacheInvalidator is a simple interface to make sure the caller of
// RetrieveAnimeRawData know what the second parameter means. The invalidator
// is used for removing an item from cache. This can be used by a parser if
// it deems an item to be invalid.
type CacheInvalidator func() error

// RetrieveAnimeRawData retrieves the HTML page for an Anime, allowing further
// processing to retrieve additional information. If any data has been found
// both a reader and an invalidator is returned. The invalidator can be used
// if whatever instance receiveing the data, deems that it is invalid an should
// be removed from cache.
func RetrieveAnimeRawData(anime *Anime) (io.ReadCloser, CacheInvalidator, error) {
	cacheIdentifier := getCacheIdentifier(anime)
	cacheFilePath := filepath.Join(cacheDir, cacheIdentifier+".html")
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

	//Ensure we don't run into recaptcha.
	rateLimiter.Wait()

	request, err := http.NewRequest(http.MethodGet, "https://proxer.me"+anime.ProxerURL, nil)
	if err != nil {
		return nil, nil, err
	}

	if loginCookieKey != "" && loginCookieValue != "" {
		request.AddCookie(&http.Cookie{
			Name:     loginCookieKey,
			Value:    loginCookieValue,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()

	cacheFileWriter, err := os.Create(cacheFilePath)
	if err != nil {
		return nil, nil, err
	}
	defer cacheFileWriter.Close()

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
