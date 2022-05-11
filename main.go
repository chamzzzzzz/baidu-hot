package main

import (
	"fmt"
	"github.com/anaskhan96/soup"
	"io/ioutil"
	"net/http"
	"strings"
)

type Hot struct {
	Title   string
	Summary string
}

type Crawler struct {
	Hots []*Hot
}

func (c *Crawler) Crawling() error {
	client := &http.Client{}
	req, err := http.NewRequest("GET", "https://top.baidu.com/board?tab=realtime", nil)
	if err != nil {
		return err
	}

	req.Header.Add("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/101.0.4951.54 Safari/537.36")
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	html, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("status code %d", res.StatusCode)
	}

	dom := soup.HTMLParse(string(html))
	if dom.Error != nil {
		return dom.Error
	}

	divs := dom.FindAllStrict("div", "class", "content_1YWBm")
	for _, div := range divs {
		divTitle := div.FindStrict("div", "class", "c-single-text-ellipsis")
		divSummary := div.Find("div", "class", "small_Uvkd3")
		if divTitle.Error != nil || divSummary.Error != nil {
			continue
		}

		title := strings.ReplaceAll(strings.Trim(divTitle.Text(), " "), "\n", "")
		summary := strings.ReplaceAll(strings.Trim(divSummary.Text(), " "), "\n", "")
		hot := &Hot{
			Title:   title,
			Summary: summary,
		}
		c.Hots = append(c.Hots, hot)
	}
	return nil
}

func main() {
	crawler := &Crawler{}
	if err := crawler.Crawling(); err != nil {
		fmt.Println("crawling hot error:", err)
		return
	}

	for i, hot := range crawler.Hots {
		fmt.Printf("%d [%s] [%s]\n", i, hot.Title, hot.Summary)
	}
}
