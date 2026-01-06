package blog_db

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/jeffereydecker/blazemarker/blaze_email"
	"github.com/jeffereydecker/blazemarker/blaze_log"
	"github.com/jeffereydecker/blazemarker/user_db"
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
	Tags      string        `json:"tags"` // Comma-separated tags
	IsNow     bool          `json:"is_now"`
	IsPrivate bool          `json:"is_private"`
	IsIndex   bool          `json:"is_index"`
}

type Reaction struct {
	gorm.Model
	ArticleID uint   `json:"article_id" gorm:"index"`
	Username  string `json:"username" gorm:"index"`
	Emoji     string `json:"emoji"`
}

type Comment struct {
	gorm.Model
	ArticleID uint   `json:"article_id" gorm:"index"`
	Username  string `json:"username" gorm:"index"`
	Content   string `json:"content" gorm:"type:text"`
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

// SearchArticlesByTag searches for articles by tag
func SearchArticlesByTag(db *gorm.DB, tag string) []Article {

	// Automatically migrate the schema
	db.AutoMigrate(&Article{})

	// Search in tags field, exclude private articles
	var articles []Article
	searchPattern := "%" + tag + "%"
	result := db.Where("(is_private = ? OR is_private IS NULL) AND tags LIKE ?",
		false, searchPattern).Find(&articles)

	if result.Error != nil {
		logger.Error("Error searching articles by tag:", "result.Error", result.Error)
	}

	return (articles)
}

// GetAllTags retrieves all unique tags from all articles
func GetAllTags(db *gorm.DB) []string {
	db.AutoMigrate(&Article{})

	var articles []Article
	db.Select("tags").Where("tags != ? AND tags IS NOT NULL", "").Find(&articles)

	// Collect all unique tags
	tagMap := make(map[string]bool)
	for _, article := range articles {
		if article.Tags != "" {
			tags := strings.Split(article.Tags, ",")
			for _, tag := range tags {
				trimmed := strings.TrimSpace(tag)
				if trimmed != "" {
					tagMap[trimmed] = true
				}
			}
		}
	}

	// Convert map to sorted slice
	var uniqueTags []string
	for tag := range tagMap {
		uniqueTags = append(uniqueTags, tag)
	}
	sort.Strings(uniqueTags)

	return uniqueTags
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

// SaveArticleWithNotifications saves an article and sends email notifications to subscribers
func SaveArticleWithNotifications(db *gorm.DB, article Article) bool {
	// First save the article
	if !SaveArticle(db, article) {
		return false
	}

	// Reload the article to get the assigned ID
	var savedArticle Article
	result := db.Where("title = ? AND author = ? AND date = ?", article.Title, article.Author, article.Date).First(&savedArticle)
	if result.Error != nil {
		logger.Error("Failed to retrieve saved article", "error", result.Error)
		return true // Article saved, but can't send notifications
	}

	// Don't send notifications for private articles
	if savedArticle.IsPrivate {
		logger.Debug("Skipping notifications for private article", "title", savedArticle.Title)
		return true
	}

	// Get all users who want notifications
	users, err := user_db.GetUsersWithNotifications(db)
	if err != nil {
		logger.Error("Failed to get users for notifications", "error", err)
		return true // Article saved successfully, just notification failed
	}

	// Send notifications asynchronously to avoid blocking
	go func() {
		articleURL := fmt.Sprintf("https://blazemarker.com/article/view/%d", savedArticle.ID)

		for _, user := range users {
			// Skip sending to the author (unless they're an admin monitoring the system)
			if user.Username == savedArticle.Author && !user.IsAdmin {
				continue
			}

			// Get author profile for display name
			authorProfile, _ := user_db.GetUserProfile(db, savedArticle.Author)
			authorName := savedArticle.Author
			if authorProfile != nil && authorProfile.Handle != "" {
				authorName = authorProfile.Handle
			}

			// Get recipient name
			recipientName := user.Handle
			if recipientName == "" {
				recipientName = user.Username
			}

			// Send email
			err := blaze_email.SendArticleNotification(
				user.Email,
				recipientName,
				savedArticle.Title,
				string(savedArticle.Content),
				articleURL,
				authorName,
			)

			if err != nil {
				logger.Error("Failed to send notification", "to", user.Email, "error", err)
			}
		}
	}()

	return true
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

// AddReaction adds a reaction to an article
func AddReaction(db *gorm.DB, articleID uint, username, emoji string) bool {
	db.AutoMigrate(&Reaction{})

	// Check if user already reacted with this emoji
	var existingReaction Reaction
	result := db.Where("article_id = ? AND username = ? AND emoji = ?", articleID, username, emoji).First(&existingReaction)

	if result.Error == nil {
		// Reaction already exists
		return true
	}

	// Create new reaction
	reaction := Reaction{
		ArticleID: articleID,
		Username:  username,
		Emoji:     emoji,
	}

	if result := db.Create(&reaction); result.Error != nil {
		logger.Error("Failed to add reaction:", "articleID", articleID, "username", username, "emoji", emoji, "error", result.Error)
		return false
	}

	logger.Info("Reaction added successfully", "articleID", articleID, "username", username, "emoji", emoji)
	return true
}

// RemoveReaction removes a reaction from an article
func RemoveReaction(db *gorm.DB, articleID uint, username, emoji string) bool {
	db.AutoMigrate(&Reaction{})

	result := db.Where("article_id = ? AND username = ? AND emoji = ?", articleID, username, emoji).Delete(&Reaction{})

	if result.Error != nil {
		logger.Error("Failed to remove reaction:", "articleID", articleID, "username", username, "emoji", emoji, "error", result.Error)
		return false
	}

	logger.Info("Reaction removed successfully", "articleID", articleID, "username", username, "emoji", emoji)
	return true
}

// GetReactions retrieves all reactions for an article, grouped by emoji
func GetReactions(db *gorm.DB, articleID uint) map[string][]string {
	db.AutoMigrate(&Reaction{})

	var reactions []Reaction
	db.Where("article_id = ?", articleID).Find(&reactions)

	// Group reactions by emoji
	reactionMap := make(map[string][]string)
	for _, reaction := range reactions {
		reactionMap[reaction.Emoji] = append(reactionMap[reaction.Emoji], reaction.Username)
	}

	return reactionMap
}

// GetUserReactions retrieves emojis a specific user reacted with for an article
func GetUserReactions(db *gorm.DB, articleID uint, username string) []string {
	db.AutoMigrate(&Reaction{})

	var reactions []Reaction
	db.Where("article_id = ? AND username = ?", articleID, username).Find(&reactions)

	var emojis []string
	for _, reaction := range reactions {
		emojis = append(emojis, reaction.Emoji)
	}

	return emojis
}

// AddComment adds a comment to an article
func AddComment(db *gorm.DB, articleID uint, username, content string) bool {
	db.AutoMigrate(&Comment{})

	comment := Comment{
		ArticleID: articleID,
		Username:  username,
		Content:   content,
	}

	if result := db.Create(&comment); result.Error != nil {
		logger.Error("Failed to add comment:", "articleID", articleID, "username", username, "error", result.Error)
		return false
	}

	logger.Info("Comment added successfully", "articleID", articleID, "username", username)
	return true
}

// GetComments retrieves all comments for an article
func GetComments(db *gorm.DB, articleID uint) []Comment {
	db.AutoMigrate(&Comment{})

	var comments []Comment
	db.Where("article_id = ?", articleID).Order("created_at asc").Find(&comments)

	return comments
}

// DeleteComment deletes a comment by ID
func DeleteComment(db *gorm.DB, commentID uint, username string) bool {
	db.AutoMigrate(&Comment{})

	result := db.Where("id = ? AND username = ?", commentID, username).Delete(&Comment{})

	if result.Error != nil {
		logger.Error("Failed to delete comment:", "commentID", commentID, "username", username, "error", result.Error)
		return false
	}

	if result.RowsAffected == 0 {
		logger.Error("Comment not found or unauthorized", "commentID", commentID, "username", username)
		return false
	}

	logger.Info("Comment deleted successfully", "commentID", commentID, "username", username)
	return true
}
