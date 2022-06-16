package proxerscrape

import (
	"bufio"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
)

func QueryRateLimited(anime *Anime) (*http.Response, error) {
	//Ensure we don't run into recaptcha.
	rateLimiter.Wait()

	request, err := http.NewRequest(http.MethodGet, "https://proxer.me"+anime.ProxerURL, nil)
	if err != nil {
		return nil, err
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
	return http.DefaultClient.Do(request)
}

// QueryProxy attempts retrieving the additional details via a proxied HTTPS
// request. However, currently this IS NOT FUNCTIONAL.
func QueryProxy(anime *Anime) (*http.Response, error) {
	request, err := http.NewRequest(http.MethodGet, "https://proxer.me"+anime.ProxerURL, nil)
	if err != nil {
		return nil, err
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

	proxyUrl := getProxyURL()
	fmt.Println(proxyUrl)
	url, err := url.Parse("https://" + proxyUrl)
	if err != nil {
		return nil, err
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(url),
		},
	}

	response, err := client.Do(request)
	if response != nil && response.Request != nil {
		fmt.Println("Hello", response.Request.Header)
	}
	return response, err
}

var proxyURLs []string
var proxyMutex *sync.Mutex

func getProxyURL() string {
	proxyMutex.Lock()
	defer proxyMutex.Unlock()
	proxy := proxyURLs[0]
	proxyURLs[0] = proxyURLs[len(proxyURLs)-1]
	proxyURLs = proxyURLs[:len(proxyURLs)-1]
	return proxy
}

func init() {
	proxyMutex = &sync.Mutex{}

	file, err := os.Open("http.txt")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	// optionally, resize scanner's capacity for lines over 64K, see next example
	for scanner.Scan() {
		proxyURLs = append(proxyURLs, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}
