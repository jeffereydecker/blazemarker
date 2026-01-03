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
	Title     string        `json:"title"`
	Content   template.HTML `json:"content"`
	Author    string        `json:"author"`
	Date      string        `json:"date"`
	IsNow     bool          `json:"is_now"`
	IsPrivate bool          `json:"is_private"`
	IsIndex   bool          `json:"is_index"`
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

	// Read all public articles only (including NULL as false)
	var articles []Article
	result := db.Where("is_private = ? OR is_private IS NULL", false).Find(&articles)
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

	// Read articles marked as Index articles (not private)
	var articles []Article
	result := db.Where("is_index = ? AND (is_private = ? OR is_private IS NULL)", true, false).Order("date DESC").Find(&articles)
	if result.Error != nil {
		logger.Error("Error reading index articles:", "result.Error", result.Error)
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

	// Read only the most recent "Now" article (not private)
	var articles []Article
	result := db.Where("is_now = ? AND (is_private = ? OR is_private IS NULL)", true, false).Order("date DESC").Limit(1).Find(&articles)

	if result.Error != nil {
		logger.Error("Error reading articles:", "result.Error", result.Error)
	}

	return (articles)
}

// GetPrivateArticles retrieves all private articles for a specific author
func GetPrivateArticles(db *gorm.DB, author string) []Article {

	// Automatically migrate the schema
	db.AutoMigrate(&Article{})

	// Read all private articles for this author
	var articles []Article
	result := db.Where("is_private = ? AND author = ?", true, author).Find(&articles)

	if result.Error != nil {
		logger.Error("Error reading private articles:", "result.Error", result.Error)
	}

	return (articles)
}

// GetMyArticles retrieves all non-private articles for a specific author
func GetMyArticles(db *gorm.DB, author string) []Article {

	// Automatically migrate the schema
	db.AutoMigrate(&Article{})

	// Read all non-private articles for this author (including NULL as false)
	var articles []Article
	result := db.Where("(is_private = ? OR is_private IS NULL) AND author = ?", false, author).Find(&articles)

	if result.Error != nil {
		logger.Error("Error reading my articles:", "result.Error", result.Error)
	}

	return (articles)
}

// SearchArticles searches for articles by keyword in title or content
func SearchArticles(db *gorm.DB, keyword string) []Article {

	// Automatically migrate the schema
	db.AutoMigrate(&Article{})

	// Search in title and content, exclude private articles
	var articles []Article
	searchPattern := "%" + keyword + "%"
	result := db.Where("(is_private = ? OR is_private IS NULL) AND (title LIKE ? OR content LIKE ?)",
		false, searchPattern, searchPattern).Find(&articles)

	if result.Error != nil {
		logger.Error("Error searching articles:", "result.Error", result.Error)
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

// GetArticleByID retrieves a single article by its ID
func GetArticleByID(db *gorm.DB, id uint) (Article, error) {
	var article Article
	result := db.First(&article, id)
	if result.Error != nil {
		logger.Error("Error reading article by ID:", "id", id, "error", result.Error)
		return Article{}, result.Error
	}
	return article, nil
}

// UpdateArticle updates an existing article in the database
func UpdateArticle(db *gorm.DB, article Article) bool {
	if result := db.Model(&article).Updates(article); result.Error != nil {
		logger.Error("Failed to update article:", "id", article.ID, "error", result.Error)
		return false
	}
	logger.Info("Article updated successfully", "id", article.ID, "title", article.Title)
	return true
}

// DeleteArticle removes an article from the database by ID
func DeleteArticle(db *gorm.DB, id uint) bool {
	if result := db.Delete(&Article{}, id); result.Error != nil {
		logger.Error("Failed to delete article:", "id", id, "error", result.Error)
		return false
	}
	logger.Info("Article deleted successfully", "id", id)
	return true
}

// UpdateArticleAndFile updates both database and JSON file
func UpdateArticleAndFile(db *gorm.DB, article Article) bool {
	// Update database
	if !UpdateArticle(db, article) {
		return false
	}

	// Update JSON file
	if !SaveArticleToFile(article) {
		logger.Error("Failed to save updated article to file", "id", article.ID, "title", article.Title)
		return false
	}

	return true
}
