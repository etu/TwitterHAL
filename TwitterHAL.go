package main

import (
	"fmt"
	"github.com/kurrik/oauth1a"
	"github.com/kurrik/twittergo"
	cobe "github.com/pteichman/go.cobe"
	"gopkg.in/gcfg.v1"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	Twitter struct {
		ConsumerKey       string
		ConsumerSecret    string
		AccessToken       string
		AccessTokenSecret string
	}
}

func main() {
	var lastTweet uint64 = 0
	var config Config

	// Load up config
	if err := gcfg.ReadFileInto(&config, "TwitterHAL.gcfg"); err != nil {
		fmt.Printf("Config error: %s\n", err)
		os.Exit(1)
	}

	// Set to a quite large value to not start counting down instantly
	var responseCountdown int = 1000

	// Set up Twitter Client
	var client *twittergo.Client = getTwitter(config)

	// Set up MegaHAL
	replyOpts := cobe.ReplyOptions{0, nil}
	megahal, _ := cobe.OpenCobe2Brain("twitterhal-brainfile")

	for true {
		// Print tweets
		for _, tweet := range fetchNewTweets(client, lastTweet) {
			fmt.Printf("\n\n======================\n")

			// Create a response to recieved tweet
			response := formatResponse(
				megahal.ReplyWithOptions(
					cleanTweetText(tweet.Text()),
					replyOpts,
				),
			)

			// Store recieved tweet in brain
			megahal.Learn(cleanTweetText(tweet.Text()))

			// Count down responseCountdown if the response is short enough
			if len(response) <= 140 {
				responseCountdown--
			}

			// Print recieved tweet
			fmt.Printf(
				"@%s: %s\n",
				tweet.User().ScreenName(),
				cleanTweetText(tweet.Text()),
			)

			// Print generated response
			fmt.Printf(
				"Response(%v): %v\n",
				len(response),
				response,
			)
			fmt.Printf("Response Countdown: %v\n", responseCountdown)

			// If countdown is below 0, send response and reset countdown
			if responseCountdown < 0 {
				// Check current hour so he doesn't tweet between 00:00-07:00
				if time.Now().Hour() > 6 {
					sendTweet(client, response)

					fmt.Println("tweet sent")
				}

				responseCountdown = 25 + rand.Intn(10)
			}

			// Counter to only fetch new tweets
			if tweet.Id() > lastTweet {
				lastTweet = tweet.Id()
			}
		}

		time.Sleep(time.Second * 60)
	}
}

func cleanTweetText(text string) string {
	text = strings.Replace(text, "\n", " ", -1)
	text = strings.Replace(text, "#", "", -1)
	text = strings.Replace(text, "(", "", -1)
	text = strings.Replace(text, ")", "", -1)
	text = strings.Replace(text, "|", "", -1)
	text = strings.Replace(text, "♥", "", -1)
	text = strings.Replace(text, "\"", "", -1)
	text = strings.Replace(text, "[", "", -1)
	text = strings.Replace(text, "]", "", -1)
	text = strings.Replace(text, "”", "", -1)
	text = strings.Replace(text, "“", "", -1)

	text = regexp.MustCompile("https?://[^ ]+").ReplaceAllString(text, "")
	text = regexp.MustCompile("@[^ ]+").ReplaceAllString(text, "")
	text = regexp.MustCompile("&.+;").ReplaceAllString(text, "")

	text = strings.Trim(text, " ")

	return text
}

func formatResponse(text string) string {
	// Make sure that svpol is in the text as hashtag
	if strings.Contains(text, "svpol") {
		text = strings.Replace(text, "svpol", "#svpol", -1)
	} else {
		text = text + " #svpol"
	}

	// Make migpol a hashtag
	if strings.Contains(text, "migpol") {
		text = strings.Replace(text, "migpol", "#migpol", -1)
	}

	// Remove double (or more) spaces
	text = regexp.MustCompile("@[^ ]+").ReplaceAllString(text, "")

	return text
}

func sendTweet(client *twittergo.Client, text string) {
	var (
		err error
		req *http.Request
	)

	// Build Params
	query := url.Values{}
	query.Set("status", text)

	// Bulid body
	body := strings.NewReader(query.Encode())

	// Prepare request
	if req, err = http.NewRequest("POST", "/1.1/statuses/update.json", body); err != nil {
		fmt.Printf("Could not parse request: %v\n", err)
		os.Exit(1)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Send Request
	if _, err = client.SendRequest(req); err != nil {
		fmt.Printf("Could not send request: %v\n", err)
		os.Exit(1)
	}
}

func fetchNewTweets(client *twittergo.Client, lastTweet uint64) []twittergo.Tweet {
	var (
		err     error
		req     *http.Request
		resp    *twittergo.APIResponse
		results *twittergo.SearchResults
	)

	// Build search
	query := url.Values{}
	query.Set("q", "#svpol -rt")
	query.Set("lang", "sv")
	query.Set("result_type", "recent")
	query.Set("count", "20")
	query.Set("since_id", fmt.Sprintf("%d", lastTweet))

	// Build URI
	url := fmt.Sprintf("/1.1/search/tweets.json?%v", query.Encode())

	// Prepare request
	if req, err = http.NewRequest("GET", url, nil); err != nil {
		fmt.Printf("Could not parse request: %v\n", err)
		os.Exit(1)
	}

	// Sign and send request
	if resp, err = client.SendRequest(req); err != nil {
		fmt.Printf("Could not send request: %v\n", err)
		os.Exit(1)
	}

	// Parse requests
	results = &twittergo.SearchResults{}
	if err = resp.Parse(results); err != nil {
		fmt.Printf("Problem parsing response: %v\n", err)
		os.Exit(1)
	}

	// Print ratelimit data
	if resp.HasRateLimit() {
		fmt.Printf("Rate limit:           %v\n", resp.RateLimit())
		fmt.Printf("Rate limit remaining: %v\n", resp.RateLimitRemaining())
		fmt.Printf("Rate limit reset:     %v\n", resp.RateLimitReset())
	}

	return results.Statuses()
}

func getTwitter(config Config) *twittergo.Client {
	twitterConfig := &oauth1a.ClientConfig{
		ConsumerKey:    config.Twitter.ConsumerKey,
		ConsumerSecret: config.Twitter.ConsumerSecret,
	}

	user := oauth1a.NewAuthorizedConfig(
		config.Twitter.AccessToken,
		config.Twitter.AccessTokenSecret,
	)

	return twittergo.NewClient(twitterConfig, user)
}
