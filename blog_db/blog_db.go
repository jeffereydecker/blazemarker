package blog_db

import (
	"encoding/json"
	"html/template"
	"os"
	"sort"

	"gorm.io/gorm"

	"github.com/jeffereydecker/blazemarker/blaze_log"
)

var logger = blaze_log.GetLogger()

type ByDate []Article

func (a ByDate) Len() int           { return len(a) }
func (a ByDate) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByDate) Less(i, j int) bool { return a[i].Date > a[j].Date } // Sorting in descending order

type Article struct {
	gorm.Model
	Title   string        `json:"title"`
	Content template.HTML `json:"content"`
	Author  string        `json:"author"`
	Date    string        `json:"date"`
	IsNow   bool          `json:"is_now"`
}

//type Article struct {
//	ID      uint          `gorm:"primaryKey" json:"id"`
//	Title   string        `json:"title"`
//	Content template.HTML `json:"content"`
//	Author  string        `json:"author"`
//	Date    string        `json:"date"`
//}

func GetAllArticlesFromFiles() []Article {
	files, err := os.ReadDir("../articles")
	if err != nil {
		logger.Error(err.Error())
		return (nil)
	}

	articles := make([]Article, 0)

	for _, file := range files {
		jsonData, err := os.ReadFile("../articles/" + file.Name())
		if err != nil {
			logger.Error(err.Error())
			continue
		}

		var article Article
		if err := json.Unmarshal(jsonData, &article); err != nil {
			logger.Error(err.Error())
			continue
		}

		articles = append(articles, article)
	}

	return (articles)
}

func GetAllArticles(db *gorm.DB) []Article {

	// Automatically migrate the schema
	db.AutoMigrate(&Article{})

	// Read all articles
	var articles []Article
	result := db.Find(&articles)
	if result.Error != nil {
		logger.Error("Error reading articles:", "result.Error", result.Error)
	}

	return (articles)
}

func GetIndexArticlesFromFile() []Article {
	jsonData, err := os.ReadFile("../articles/2024-07-13Welcome to Blazemarkerjdecker.json")
	if err != nil {
		logger.Error(err.Error())
		return nil
	}

	var article Article
	if err := json.Unmarshal(jsonData, &article); err != nil {
		logger.Error(err.Error())
		return nil
	}

	articles := make([]Article, 0)
	article.ID = 0

	articles = append(articles, article)

	return articles

}

func GetIndexArticles(db *gorm.DB) []Article {

	// Automatically migrate the schema
	db.AutoMigrate(&Article{})

	// Read all articles
	var articles []Article
	result := db.Where("Title =?", "Welcome to Blazemarker").Find(&articles)
	if result.Error != nil {
		logger.Error("Error reading articles:", "result.Error", result.Error)
	}

	return (articles)
}

func GetNowArticlesFromFile() []Article {
	jsonData, err := os.ReadFile("../articles/2024-07-13What I'm Doing Nowjdecker.json")
	if err != nil {
		logger.Error(err.Error())
		return nil
	}

	var article Article
	if err := json.Unmarshal(jsonData, &article); err != nil {
		logger.Error(err.Error())
		return nil
	}

	articles := make([]Article, 0)

	articles = append(articles, article)

	return articles

}

func GetNowArticles(db *gorm.DB) []Article {

	// Automatically migrate the schema
	db.AutoMigrate(&Article{})

	// Read all articles where IsNow is true
	var articles []Article
	result := db.Where("is_now = ?", true).Find(&articles)

	if result.Error != nil {
		logger.Error("Error reading articles:", "result.Error", result.Error)
	}

	return (articles)
}

func SortByDate(articles []Article) {
	sort.Sort(ByDate(articles))
}

func SaveArticleToFile(article Article) bool {

	// Marshal blog entry struct to JSON
	jsonData, err := json.MarshalIndent(article, "", "    ")
	if err != nil {
		logger.Error(err.Error())
		return (false)
	}

	// Write JSON data to file
	filename := "../articles/" + article.Date + article.Title + article.Author + ".json"
	err = os.WriteFile(filename, jsonData, 0644)
	if err != nil {
		logger.Error(err.Error())
		return (false)
	}

	return (true)
}

func SaveArticle(db *gorm.DB, article Article) bool {
	if result := db.Create(&article); result.Error != nil {
		logger.Error("Failed to create article:", "result.Error", result.Error)
		return false
	}

	return (true)
}
