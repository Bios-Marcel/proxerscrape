package proxerscrape

import (
	"net/http"
)

func QueryDirectly(url string) (*http.Response, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if loginCookieKey != "" && loginCookieValue != "" {
		request.AddCookie(&http.Cookie{
			Name:     loginCookieKey,
			Value:    loginCookieValue,
			Path:     "/",
			Domain:   "proxer.me",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
		})
	}

	//NOTE Adding the cookies for showing tags here doesn't work.

	return http.DefaultClient.Do(request)
}
