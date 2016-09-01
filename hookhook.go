package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	resultsPerPage       = 50
	resultsPerRedditPage = 25
	cacheDir             = "./results"
	maxPhotoRetries      = 5
	nakedThreshold       = 3
	maxRedditPages       = 20
	maxRedditRetries     = 5
)

var (
	maxPages       = 5
	debug          *bool
	bingAccountKey *string
	slackHook      string
	query          string
	botName        string
	message        string
	subreddits     = "girlswithglasses+Stacked+gonewild+realgirls+curvy+bustypetite+nsfw_gifs+Burstingout+bonermaterial+facedownassup+pawg+girlsinmessyrooms"
)

type (
	QueryResult struct {
		D *D `json:"d"`
	}

	D struct {
		Results []Result `json:"results"`
		Next    string   `json:"__next"`
	}

	Result struct {
		MetaData    *ResMeta `json:"__metadata"`
		ID          string
		Title       string
		MediaUrl    string
		SourceUrl   string
		DisplayUrl  string
		Width       string
		Height      string
		FileSize    string
		ContentType string
		Thumbnail   *Thumb
		Sent        bool
	}

	ResMeta struct {
		Uri  string `json:"uri"`
		Type string `json:"type"`
	}

	Thumb struct {
		MetaData    *ResMeta `json:"__metadata"`
		MediaUrl    string
		ContentType string
		Width       string
		Height      string
		FileSize    string
	}
)

func main() {
	queryPtr := flag.String("q", "girl boobs", "Search query")
	botNamePtr := flag.String("n", "commit-bot", "Bot name")
	bingAccountKey = flag.String("b", "", "Bing account key")
	hookUrlPtr := flag.String("h", "", "Slack hook url")
	maxPagesPtr := flag.Int("p", maxPages, "Max pages")
	msgPtr := flag.String("m", "#titsforcommits", "Slack message")
	subredditsPtr := flag.String("r", subreddits, "Subreddits")
	debug = flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	query = *queryPtr
	botName = *botNamePtr
	slackHook = *hookUrlPtr
	maxPages = *maxPagesPtr
	message = *msgPtr
	subreddits = *subredditsPtr

	// Random number seed
	rand.Seed(time.Now().UTC().UnixNano())

	if slackHook == "" {
		fmt.Println("INFO: No Slack hook URL given (-h). Each request requires hook URL.")
	}
	if *bingAccountKey == "" {
		fmt.Println("WARN: No Bing account key given (-b). Using reddit sources only.")
	}

	port := 6969

	fmt.Printf("Serving on http://localhost:%d\n", port)

	http.HandleFunc("/", hookIt)
	http.HandleFunc("/get/reddit", getReddit)
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

func lineCounter(r io.Reader) (int, error) {
	buf := make([]byte, 8196)
	count := 0
	lineSep := []byte{'\n'}

	for {
		c, err := r.Read(buf)
		if err != nil && err != io.EOF {
			return count, err
		}

		count += bytes.Count(buf[:c], lineSep)

		if err == io.EOF {
			break
		}
	}

	return count, nil
}

func getPayload(r *http.Request) (*Payload, error) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Printf("Error reading payload: %s\n", err)
		return nil, errors.New("Error reading payload.")
	}

	isGitHubHook := len(body) > 0
	if isGitHubHook {
		pl := new(Payload)
		if err = json.Unmarshal(body, &pl); err != nil {
			fmt.Printf("GitHub hook decoder err: %s\n", err)
			fmt.Printf("%s\n", body)
			return nil, errors.New("GitHub hook decoder error.")
		}

		committer := pl.HeadCommit.Committer.Username
		if committer == "" {
			committer = pl.HeadCommit.Committer.Name
		}
		fmt.Printf("GitHub push detected!\nBranch:%s\nAuthor:%s\nCommits:%d\n", pl.Ref, committer, len(pl.Commits))

		return pl, nil
	}

	return nil, nil
}

func oneLineCommitMsg(msg string) string {
	nl := strings.Index(msg, "\n")
	if nl > -1 {
		return msg[:nl]
	}

	return msg
}

func lowerFirst(s string) string {
	if s == "" {
		return ""
	}
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[n:]
}

func escapeQuotes(s string) string {
	return strings.Replace(s, "\"", "\\\"", -1)
}

var templates = template.Must(template.ParseFiles("templates/boobs.html", "templates/slacker.html"))

type Page struct {
	Title       string
	Description string
	Pic         string
	SourceUrl   string
}

func getReddit(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{}

	var pic string
	sourcePic := ""

	rand.Seed(time.Now().UTC().UnixNano())

	jump := rand.Intn(maxRedditPages)
	pic, sourcePic = searchRedditSkipping(jump, true, client)
	if strings.Contains(pic, "imgur") && !strings.HasSuffix(pic, ".jpg") {
		pic += ".jpg"
	} else if strings.HasPrefix(pic, "https://thumbs.gfycat.com") {
		// Get huge GIF from gfycat
		pic = strings.Replace(pic, "https://thumbs.gfycat.com", "https://giant.gfycat.com", -1)
		pic = strings.Replace(pic, "-thumb100.jpg", ".gif", -1)
	}

	p := &Page{Title: "Best bewbs 4 u", Description: "Free boobs.", Pic: pic, SourceUrl: sourcePic}

	templates.ExecuteTemplate(w, "boobs", p)
}

func hookIt(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		templates.ExecuteTemplate(w, "slacker", nil)
		return
	}

	// Validate request
	if r.Method != "POST" {
		return
	}

	hook := r.FormValue("hook")
	if slackHook != "" {
		hook = slackHook
	}
	if hook == "" {
		respondError(w, http.StatusBadRequest, "Need a Slack hook URL to post this to.\nSee http://slacker.fart.attorney for instructions.")
		return
	}

	if *debug {
		fmt.Println("======\n")
	}

	// Read in GitHub payload, if any
	pl, _ := getPayload(r)
	getNaked := pl != nil && len(pl.Commits) >= nakedThreshold

	// Potentially override server default query with requested one
	q := r.FormValue("q")
	if q == "" {
		q = query
	}
	if *debug {
		fmt.Printf("DEBUG: Query: %s\n", q)
	}
	q = url.QueryEscape(q)

	// Use given channel
	ch := r.FormValue("chan")
	if getNaked {
		ch = "general"
	}
	if *debug {
		fmt.Printf("DEBUG: Channel: %s\n", ch)
	}

	// Potentially override server default message in Slack
	msg := r.FormValue("msg")
	if msg == "" {
		msg = message
	}
	if pl != nil {
		committer := pl.HeadCommit.Committer.Username
		if committer == "" {
			committer = pl.HeadCommit.Committer.Name
		}

		if getNaked {
			msg = fmt.Sprintf("So productive, %s! I love when you %s, %s, and especially %s!", committer, oneLineCommitMsg(lowerFirst(pl.Commits[0].Message)), oneLineCommitMsg(lowerFirst(pl.Commits[1].Message)), oneLineCommitMsg(lowerFirst(pl.Commits[2].Message)))
		} else {
			cMsg := oneLineCommitMsg(lowerFirst(pl.HeadCommit.Message))
			compCMsg := strings.ToLower(cMsg)
			if strings.HasPrefix(compCMsg, "revert") {
				msg = fmt.Sprintf("Don't undo our love, %s, don't %s :broken_heart:", committer, cMsg)
			} else if strings.Contains(compCMsg, "copy") && strings.Contains(compCMsg, "change") {
				msg = fmt.Sprintf("OMG %s! So hawt when you %s!!! :heart_eyes:", committer, cMsg)
			} else if strings.Contains(compCMsg, "color") {
				msg = fmt.Sprintf("Oh %s I :heart: every time you %s!", committer, cMsg)
			} else if strings.Contains(compCMsg, "caching") || strings.Contains(compCMsg, "cache") {
				msg = fmt.Sprintf("Oh :dollar: %s :dollar: show me how you %s! :dollar:", committer, cMsg)
			} else if strings.Contains(compCMsg, "fix") {
				msg = fmt.Sprintf("I love a man like %s who can %s! :kissing_heart:", committer, cMsg)
			} else if strings.HasPrefix(compCMsg, "merge") {
				msg = fmt.Sprintf(":kissing_heart: %s, %s!", cMsg, committer)
			} else {
				msg = fmt.Sprintf("Oh %s! I love when you %s!", committer, cMsg)
			}
		}
	}
	if *debug {
		fmt.Printf("DEBUG: Message: %s\n", msg)
	}

	// Potentially override server default bot name in Slack
	botN := r.FormValue("bot")
	if botN == "" {
		botN = botName
	}
	if *debug {
		fmt.Printf("DEBUG: Bot name: %s\n", botN)
	}

	ic := r.FormValue("icon")
	if ic == "" {
		ic = "octocat"
	}

	adult := r.FormValue("a")
	if adult == "" {
		// No preference explicitly given, so turn off SafeSearch if earned
		if getNaked {
			adult = "Off"
		} else {
			adult = "Moderate"
		}
	}

	source := r.FormValue("source")

	// Pick a random page of search results
	skip := strconv.Itoa(rand.Intn(maxPages) * resultsPerPage)

	client := &http.Client{}

	var pic string
	sourcePic := ""
	isRedditSrc := *bingAccountKey == "" || pl != nil || source == "reddit"
	if isRedditSrc {
		// If we have a GitHub payload, look to reddit instead of Bing
		jump := rand.Intn(maxRedditPages)
		pic, sourcePic = searchRedditSkipping(jump, adult == "Off", client)
		if strings.Contains(pic, "imgur") && !strings.HasSuffix(pic, ".jpg") {
			pic += ".jpg"
		}
	} else {
		// TODO: put search - check - retry in a function
		pic = search(q, skip, adult, client)
		if pic == "" {
			// There was some kind of error
			w.WriteHeader(500)
			return
		}

		if *debug {
			fmt.Printf("DEBUG: Pic: %s\n", pic)
		}
	}

	// Make sure the image is valid
	forceRetry := false
	var contentType string

	req, err := http.NewRequest("HEAD", pic, nil)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("Couldn't fetch image: ", err)
		forceRetry = true
		contentType = ""
	} else {
		defer resp.Body.Close()

		contentType = resp.Header.Get("Content-Type")
		if *debug {
			fmt.Println("DEBUG: Pic Status:", resp.Status)
			fmt.Println("DEBUG: Pic Type:", contentType)
		}
	}

	if !isRedditSrc && (forceRetry || resp.StatusCode != http.StatusOK || !strings.HasPrefix(contentType, "image")) {
		forceRetry = false
		retryCount := 1
		for ((resp == nil || resp.StatusCode != http.StatusOK) || !strings.HasPrefix(contentType, "image")) && retryCount <= maxPhotoRetries {
			if *debug {
				fmt.Printf("DEBUG: Retrying %d... ", retryCount)
			}

			pic = search(q, skip, adult, client)
			if pic == "" {
				// There was some kind of error
				w.WriteHeader(500)
				return
			}

			if *debug {
				fmt.Printf("DEBUG: %s\n", pic)
			}

			req, err = http.NewRequest("HEAD", pic, nil)
			resp, err = client.Do(req)
			if err != nil {
				panic(err)
			}
			defer resp.Body.Close()

			contentType = resp.Header.Get("Content-Type")
			if *debug {
				fmt.Println("DEBUG: Pic Status:", resp.Status)
				fmt.Println("DEBUG: Pic Type:", contentType)
			}

			retryCount++
		}
	}

	if sourcePic == "" {
		sourcePic = pic
	}

	// Prepare JSON post to Slack
	jsonStr := fmt.Sprintf("{\"response_type\": \"in_channel\", \"username\":\"%s\",\"icon_emoji\":\":%s:\",\"channel\":\"%s\",\"attachments\":[{\"fallback\":\"%s - %s\",\"pretext\":\"%s\",\"text\":\"<%s|Source>\",\"color\":\"#eee\",\"image_url\":\"%s\"}]}", botN, ic, ch, escapeQuotes(msg), pic, escapeQuotes(msg), sourcePic, pic)
	if *debug {
		fmt.Printf("DEBUG: %s\n\n", jsonStr)
	}

	// Hit slack hook
	var json = []byte(jsonStr)
	req, err = http.NewRequest("POST", hook, bytes.NewBuffer(json))
	req.Header.Set("Content-Type", "application/json")

	resp, err = client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	if *debug {
		fmt.Println("DEBUG: response Status:", resp.Status)
	}
	body, _ := ioutil.ReadAll(resp.Body)
	if *debug {
		fmt.Println("DEBUG: response Body:", string(body), "\n")
	}
}

func fetchReddit(after string, client *http.Client) *Subreddit {
	var sr = new(Subreddit)
	redditUrl := fmt.Sprintf("https://www.reddit.com/r/%s/.json?after=%s", subreddits, after)
	if *debug {
		fmt.Printf("DEBUG: %s\n", redditUrl)
	}

	req, err := http.NewRequest("GET", redditUrl, nil)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("GET err: %s\n", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 503 {
		return &Subreddit{Code: 503}
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("read resp err: %s\n", err)
		return nil
	}

	if err := json.Unmarshal(b, &sr); err != nil {
		fmt.Printf("decoder err: %s\n", err)
		fmt.Printf("%s\n", b)
		return nil
	}

	return sr
}

func searchRedditSkipping(skip int, adult bool, client *http.Client) (string, string) {
	// Skip a certain number of pages
	afterId := ""
	var sr *Subreddit

	cacheFile := fmt.Sprintf("%s/%s|%d.json", cacheDir, subreddits, skip)
	if _, err := os.Stat(cacheFile); err == nil {
		if *debug {
			fmt.Printf("DEBUG: subreddits %s cached, skip %d\n", subreddits, skip)
		}

		// Cache exists, so use that
		if err := json.Unmarshal(ReadData(cacheFile), &sr); err != nil {
			fmt.Println(err)
		}
	} else {
		for i := 1; i < skip; i++ {
			sr = fetchReddit(afterId, client)
			if sr == nil {
				return "https://www.redditstatic.com/trouble-afoot.jpg", "http://reddit.com"
			}
			afterId = sr.Data.After
		}

		if sr != nil {
			if b, err := json.Marshal(&sr); err != nil {
				fmt.Printf("decoder err: %s\n", err)
				fmt.Printf("%s\n", b)
			} else {
				// Save result locally
				WriteData(cacheFile, b)
			}
		}
	}

	return searchReddit(sr, adult, client)
}

func searchReddit(sr *Subreddit, adult bool, client *http.Client) (string, string) {
	retries := 0
	for (sr == nil || sr.Data == nil) && retries < maxRedditRetries {
		sr = fetchReddit("", client)
		retries++
	}
	if sr.Code == 503 {
		return "https://www.redditstatic.com/trouble-afoot.jpg", "http://reddit.com"
	}
	if sr.Data == nil {
		return "", ""
	}

	maxRes := len(sr.Data.Children)
	i := rand.Intn(maxRes)

	picItem := sr.Data.Children[i].Data
	if adult && !picItem.Over18 {
		fmt.Sprintf("image wasn't over 18 when we want adult. Retrying...\n")
		return searchReddit(sr, adult, client)
	}

	if picItem.Media != nil {
		return picItem.Media.Oembed.ThumbnailUrl, picItem.Url
	}
	return picItem.Url, picItem.Url
}

func search(query, skip, adult string, client *http.Client) string {
	var qr = new(QueryResult)
	cacheFile := cacheDir + "/" + query + "|" + skip + "|" + adult + ".json"

	if _, err := os.Stat(cacheFile); err == nil {
		if *debug {
			fmt.Printf("DEBUG: Cached: %s, adult: %s, skip %s\n", query, adult, skip)
		}

		// Cache exists, so use that
		if err := json.Unmarshal(ReadData(cacheFile), &qr); err != nil {
			fmt.Println(err)
		}
	} else {
		// No cache, so get search results
		if *debug {
			fmt.Print("DEBUG: NOT cached: ")
		}
		searchUrl := fmt.Sprintf("https://api.datamarket.azure.com/Bing/Search/v1/Image?$format=json&$skip=%s&Query='%s'&Adult='%s'", skip, query, adult)
		if *debug {
			fmt.Printf("DEBUG: %s\n", searchUrl)
		}

		req, err := http.NewRequest("GET", searchUrl, nil)
		req.SetBasicAuth(*bingAccountKey, *bingAccountKey)
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("GET err: %s\n", err)
			return ""
		}
		defer resp.Body.Close()

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Printf("read resp err: %s\n", err)
			return ""
		}

		//err = json.NewDecoder(b).Decode(qr)
		if err := json.Unmarshal(b, &qr); err != nil {
			fmt.Printf("decoder err: %s\n", err)
			fmt.Printf("%s\n", b)
			return ""
		}

		// Save result locally
		WriteData(cacheFile, b)
	}

	// Pick a random result
	maxRes := len(qr.D.Results)
	i := rand.Intn(maxRes)
	tryNum := 1
	tryMap := map[int]bool{}
	tryMap[i] = true
	for tryNum <= maxRes && qr.D.Results[i].Sent {
		tryNum++
		// Keep looking for results that haven't been sent yet
		i = rand.Intn(maxRes)
		if tryMap[i] {
			// Already tried, skip
			continue
		}
		tryMap[i] = true
	}

	if tryNum == maxRes {
		// TODO: return something!
	}

	// Remember that this was sent
	qr.D.Results[i].Sent = true

	o, err := json.Marshal(qr)
	if err != nil {
		fmt.Printf("encoder err: %s\n", err)
		fmt.Printf("%s\n", o)
		return ""
	}

	// Update cache with "sent" value
	WriteData(cacheFile, o)

	return qr.D.Results[i].MediaUrl
}

func respondError(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	fmt.Fprintf(w, msg)
}
