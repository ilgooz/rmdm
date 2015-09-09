package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/mrjones/oauth"
)

var keysFilePath = flag.String("keys", "keys.txt", "Path of Keys File")

type DM struct {
	Id int64 `json:"id_str,string"`
}

type Key []string

var noTextRegexp = regexp.MustCompile(`^[ \t]*$`)

func getKeys() []Key {
	var keys []Key

	file, err := os.Open(*keysFilePath)
	if err != nil {
		log.Fatalln("keys file not found")
	}
	defer file.Close()

	r := bufio.NewReader(file)

	var key Key

	for {
		line, _, err := r.ReadLine()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatalln(err)
		}

		if !noTextRegexp.Match(line) {
			key = append(key, string(line))

			if len(key) == 4 {
				keys = append(keys, key)
			}
		} else {
			key = Key{}
		}
	}

	return keys
}

func makeClients() []*http.Client {
	var clients []*http.Client

	for _, key := range getKeys() {
		consumer := oauth.NewConsumer(
			key[0],
			key[1],
			oauth.ServiceProvider{
				RequestTokenUrl:   "https://api.twitter.com/oauth/request_token",
				AuthorizeTokenUrl: "https://api.twitter.com/oauth/authorize",
				AccessTokenUrl:    "https://api.twitter.com/oauth/access_token",
			})

		at := oauth.AccessToken{
			Token:  key[2],
			Secret: key[3],
		}

		client, err := consumer.MakeHttpClient(&at)
		if err != nil {
			log.Fatalln(err)
		}
		clients = append(clients, client)
	}

	return clients
}

func main() {
	flag.Parse()
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	clients := makeClients()
	fmt.Println(fmt.Sprintf("total apps: %d", len(clients)))

	var (
		recentDeleted    int64
		recentOwnDeleted int64
	)

LOOP:

	for {
		for _, client := range clients {
			rate, err := deleteDMs("direct_messages", recentDeleted, client)
			if err != nil {
				log.Fatalln(err)
			}
			if rate {
				continue
			}

			rate, err = deleteDMs("direct_messages/sent", recentOwnDeleted, client)
			if err != nil {
				log.Fatalln(err)
			}
			if rate {
				continue
			}

			fmt.Println("all clear shuting down")
			break LOOP
		}

		fmt.Println("all apps are rate limited, will run again after 16m")
		time.Sleep(time.Minute * 16)
	}
}

type RateErrResponse struct {
	rate bool
	err  error
}

func deleteDMs(endpoint string, maxId int64, c *http.Client) (bool, error) {
	for {
		fmt.Println("scanning for " + endpoint)

		rate, ids, err := getDMIds(endpoint, maxId, c)
		if rate || err != nil {
			return rate, err
		}

		l := len(ids)

		if l > 0 {
			fmt.Println(fmt.Sprintf("%d DMs will be deleted", l))

			responseC := make(chan RateErrResponse, l)

			for _, id := range ids {
				go func(id int64) {
					rate, err := deleteDM(id, c)
					responseC <- RateErrResponse{rate, err}
				}(id)
			}

			for ; l > 0; l-- {
				r := <-responseC
				if r.err != nil {
					log.Println(fmt.Sprintf("DM delete err: %v", err))
					continue
				}
				if r.rate {
					return r.rate, nil
				}
			}

			fmt.Println("deleted")
		} else {
			fmt.Println("there is no DMs to delete")
			return false, nil
		}
	}
}

func getDMIds(endpoint string, maxId int64, c *http.Client) (bool, []int64, error) {
	var ids []int64

	params := url.Values{
		"count": []string{"200"},
	}
	if maxId != 0 {
		params["max_id"] = []string{fmt.Sprintf("%d", maxId)}
	}

	resp, err := c.Get("https://api.twitter.com/1.1/" + endpoint + ".json?" + params.Encode())
	if err != nil {
		return false, ids, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return true, ids, nil
	}

	if resp.StatusCode != 200 {
		return false, ids, fmt.Errorf("twitter(/1.1/direct_messages.json) status code: %d", resp.StatusCode)
	}

	var dms []DM

	err = json.NewDecoder(resp.Body).Decode(&dms)
	if err != nil {
		return false, ids, err
	}

	for _, dm := range dms {
		ids = append(ids, dm.Id)
	}

	return false, ids, nil
}

func deleteDM(id int64, c *http.Client) (bool, error) {
	resp, err := c.PostForm(
		"https://api.twitter.com/1.1/direct_messages/destroy.json",
		url.Values{"id": []string{fmt.Sprintf("%d", id)}})
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return true, nil
	}

	if resp.StatusCode != 200 {
		return false, fmt.Errorf("twitter(/1.1/direct_messages.json) status code: %d", resp.StatusCode)
	}

	return false, nil
}
