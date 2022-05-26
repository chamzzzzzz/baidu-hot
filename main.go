package main

import (
	"database/sql"
	"fmt"
	"github.com/anaskhan96/soup"
	_ "github.com/mattn/go-sqlite3"
	"github.com/robfig/cron/v3"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type Hot struct {
	Title   string
	Summary string
	Date    string
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

func (c *Crawler) SaveToFile(filePath string) error {
	content := ""
	for _, hot := range c.Hots {
		content += fmt.Sprintf("%s\n%s\n", hot.Title, hot.Summary)
	}
	return os.WriteFile(filePath, []byte(content), 0666)
}

type Archive struct {
	Source      string
	Count       int
	IgnoreCount int
}

type Archiver struct {
	db         *sql.DB
	selectStmt *sql.Stmt
	insertStmt *sql.Stmt
	Archives   []*Archive
}

func (a *Archiver) Archiving() error {
	if err := a.prepare(); err != nil {
		return err
	}

	files, err := ioutil.ReadDir("./")
	if err != nil {
		return err
	}

	var fileNames []string
	for _, file := range files {
		fileNames = append(fileNames, file.Name())
	}
	sort.Strings(fileNames)

	if len(fileNames) == 0 {
		return nil
	}

	if err := os.Mkdir("./archived", 0750); err != nil && !os.IsExist(err) {
		return err
	}

	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, fileName := range fileNames {
		if !strings.HasSuffix(fileName, ".hot.txt") {
			continue
		}

		date := strings.ReplaceAll(fileName, ".hot.txt", "")
		if _, err := time.Parse("2006-01-02-15-04-05", date); err != nil {
			return err
		}

		data, err := os.ReadFile(fileName)
		if err != nil {
			return err
		}

		lines := strings.Split(string(data), "\n")
		count := len(lines) / 2
		ignoreCount := 0
		for i := 0; i < count; i++ {
			hot := &Hot{
				Title:   lines[i*2],
				Summary: lines[i*2+1],
				Date:    date,
			}

			if err, ignore := a.insert(tx, hot); err != nil {
				return err
			} else if ignore {
				ignoreCount++
			}
		}

		archive := &Archive{}
		archive.Source = fileName
		archive.Count = count
		archive.IgnoreCount = ignoreCount
		a.Archives = append(a.Archives, archive)

		if err := os.Rename(archive.Source, "./archived/"+archive.Source); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	return nil
}

func (a *Archiver) prepare() error {
	db, err := sql.Open("sqlite3", "hot.sqlite")
	if err != nil {
		return err
	}

	_, err = db.Exec("CREATE TABLE IF NOT EXISTS hot (date TEXT, title TEXT, summary TEXT);")
	if err != nil {
		return err
	}

	selectStmt, err := db.Prepare("SELECT date, title FROM hot WHERE title = ?")
	if err != nil {
		return err
	}

	insertStmt, err := db.Prepare("INSERT INTO hot(date, title, summary) VALUES(?,?,?)")
	if err != nil {
		return err
	}

	a.db = db
	a.selectStmt = selectStmt
	a.insertStmt = insertStmt
	return nil
}

func (a *Archiver) insert(tx *sql.Tx, hot *Hot) (error, bool) {
	rows, err := tx.Stmt(a.selectStmt).Query(hot.Title)
	if err != nil {
		return err, false
	}
	defer rows.Close()

	for rows.Next() {
		var date, title string
		err := rows.Scan(&date, &title)
		if err != nil {
			return err, false
		}

		firstTime, err := time.Parse("2006-01-02-15-04-05", date)
		if err != nil {
			return err, false
		}

		latestTime, err := time.Parse("2006-01-02-15-04-05", hot.Date)
		if err != nil {
			return err, false
		}

		duration := latestTime.Sub(firstTime)
		if duration < 24*7*time.Hour {
			return nil, true
		}
	}

	if _, err := tx.Stmt(a.insertStmt).Exec(hot.Date, hot.Title, hot.Summary); err != nil {
		return err, false
	}

	return nil, false
}

func archiving() int {
	archiver := &Archiver{}

	if err := archiver.Archiving(); err != nil {
		fmt.Println("archiving error:", err)
		return 1
	}

	for _, archive := range archiver.Archives {
		fmt.Printf("archive %s %d/%d/%d\n", archive.Source, archive.Count-archive.IgnoreCount, archive.IgnoreCount, archive.Count)
	}
	fmt.Println("archiving finished")
	return 0
}

func crawling() int {
	crawler := &Crawler{}

	if err := crawler.Crawling(); err != nil {
		fmt.Println("crawling error:", err)
		return 1
	}

	loc, err := time.LoadLocation("Asia/Chongqing")
	if err != nil {
		fmt.Println("load location error:", err)
		return 1
	}

	now := time.Now().In(loc)
	filePath := fmt.Sprintf("%d-%02d-%02d-%02d-%02d-%02d.hot.txt", now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second())
	if err := crawler.SaveToFile(filePath); err != nil {
		fmt.Println("save to file error:", err)
		return 1
	}
	fmt.Println("crawling finished")
	return 0
}

func scheduling() int {
	loc, err := time.LoadLocation("Asia/Chongqing")
	if err != nil {
		fmt.Println("scheduling load location error:", err)
		return 1
	}

	logger := cron.VerbosePrintfLogger(log.New(os.Stdout, "cron: ", log.LstdFlags))
	c := cron.New(
		cron.WithLocation(loc),
		cron.WithLogger(logger),
		cron.WithChain(cron.Recover(logger), cron.SkipIfStillRunning(logger)),
	)

	c.AddFunc("5 * * * *", func() {
		crawling()
		archiving()
	})
	c.Run()
	return 0
}

func main() {
	if len(os.Args) <= 1 {
		os.Exit(crawling())
	}

	if os.Args[1] == "archive" {
		os.Exit(archiving())
	}

	if os.Args[1] == "schedule" {
		os.Exit(scheduling())
	}

	fmt.Println("invalid args!")
	os.Exit(1)
}
