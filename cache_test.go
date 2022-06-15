package proxerscrape

import "testing"

func Test_getCacheIdentifier(t *testing.T) {
	result := getCacheIdentifier(&Anime{
		ProxerURL: "/info/296#top",
	})
	if result != "296" {
		t.Errorf("Result = %s, instead of 296", result)
	}
}
