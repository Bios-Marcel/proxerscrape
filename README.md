# Proxer Profile Scraper

For now this only takes the `Anime` section of the profile as an HTML file and parses the four tables into arrays of animes.

There's a simple file that can show you what you still "have" to watch and how long it'll take. For example to run it on my profile, you could do the following:

```
curl https://proxer.me/user/252835/anime | go run .
```
