package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/sendgrid/sendgrid-go"
	"io"
	"net/http"
	"sync"
	"time"
)

var url = "https://vancouver.craigslist.ca/jsonsearch/apa/?postedToday=1&max_price=2000"
var baseClusterUrl = "https://vancouver.craigslist.ca"

var sg = sendgrid.NewSendGridClientWithApiKey("your sendgrid apikey.")

func main() {
	fmt.Println("Loading initial properties.")
	lastResults := loadProperties(url)
	fmt.Println("Done loading initial properties.")
	for {
		fmt.Println("Waiting 5 minutes...")
		time.Sleep(5 * time.Minute)

		newResults := loadProperties(url)
		compareResults(lastResults, newResults)

		lastResults = newResults
	}
}

func compareResults(oldR, newR map[string]map[string]interface{}) {
	newProps := make([]map[string]interface{}, 0)

	for id, prop := range newR {
		if _, ok := oldR[id]; ok {
			// Already exists.
			continue
		}
		newProps = append(newProps, prop)
	}

	newProps = filter(newProps)

	fmt.Printf("Found %d new properties.\n", len(newProps))
	if len(newProps) > 0 {
		sendNotification(newProps)
	}
}

func filter(props []map[string]interface{}) []map[string]interface{} {
	newProps := make([]map[string]interface{}, 0)

	lat0 := 49.286396
	lon0 := -123.142267
	lat1 := 49.258344
	lon1 := -123.095723

	for _, prop := range props {
		lat := prop["Latitude"].(float64)
		lon := prop["Longitude"].(float64)

		fmt.Printf("%f, %f", lat, lon)

		if lat <= lat0 && lat >= lat1 && lon >= lon0 && lon <= lon1 {
			newProps = append(newProps, prop)
		}
	}

	return newProps
}

func sendNotification(props []map[string]interface{}) {
	message := sendgrid.NewMail()
	message.AddTo("irwin.billing@gmail.com")
	message.AddToName("Irwin")
	message.SetSubject("New rentals on craigslist")
	message.SetFrom("irwin.billing@gmail.com")

	var buffer bytes.Buffer
	for _, prop := range props {
		buffer.WriteString(prop["PostingURL"].(string))
		buffer.WriteByte('\n')
	}

	message.SetText(buffer.String())

	if err := sg.Send(message); err != nil {
		fmt.Printf("Error sending email: %v\n")
		return
	}

	fmt.Printf("Done sending email.\n")
}

func loadProperties(url string) map[string]map[string]interface{} {
	results := make(map[string]map[string]interface{})

	fmt.Printf("Getting url %v\n", url)
	resp, err := http.Get(url)
	fmt.Printf("Got response")

	if err != nil {
		fmt.Printf("Error performing request: %v", err)
		return nil
	}

	var result []interface{}
	dec := json.NewDecoder(resp.Body)
	if err = dec.Decode(&result); err != nil {
		if err != io.EOF {
			fmt.Printf("Error decoding response: %v\n", err)
			return nil
		}
	}

	properties := result[0].([]interface{})
	clusters := make([]string, 0)

	for _, val := range properties {
		prop := val.(map[string]interface{})
		if url, ok := prop["url"]; ok {
			clusters = append(clusters, url.(string))
			continue
		}
		results[prop["PostingID"].(string)] = prop
	}

	wg := sync.WaitGroup{}
	wg.Add(len(clusters))

	mutex := sync.Mutex{}

	for _, cluster := range clusters {
		go func() {
			defer wg.Done()
			props := loadProperties(baseClusterUrl + cluster)

			mutex.Lock()

			for _, prop := range props {
				results[prop["PostingID"].(string)] = prop
			}

			mutex.Unlock()
		}()
	}

	wg.Wait()

	return results
}
