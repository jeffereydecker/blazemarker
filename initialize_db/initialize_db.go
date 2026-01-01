package main

import (
	"log"
	"log/slog"
	"os/user"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jeffereydecker/blazemarker/blaze_db"
	"github.com/jeffereydecker/blazemarker/blaze_log"
	"github.com/jeffereydecker/blazemarker/blog_db"
	"github.com/jeffereydecker/blazemarker/gallery_db"
)

var logger *slog.Logger = blaze_log.GetLogger()

var db *gorm.DB = blaze_db.GetDB()

// Aliases
type Article = blog_db.Article
type Photo = gallery_db.Photo
type Album = gallery_db.Album

func initializeDB() {
	// Open SQLite database
	var err error
	db, err = gorm.Open(sqlite.Open("../data/blazemarker.db"), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	// Migrate the schema
	db.AutoMigrate(&Article{})
	db.AutoMigrate(&Album{}, &Photo{})
}

func migrateArticlesToDB() {
	Articles := blog_db.GetAllArticlesFromFiles()
	for _, article := range Articles {
		logger.Info("Another article", "article.ID", article.ID)
		if result := db.Create(&Article{Title: article.Title, Content: article.Content, Date: article.Date}); result.Error != nil {
			logger.Error("Failed to create article:", "result.Error", result.Error)
			return
		}
	}

}

func migratePhotosToDB() {
	albums := gallery_db.GetAllAlbumsFromFiles()

	for _, album := range albums {
		logger.Info("Another album", "album.ID", album.ID)

		if result := db.Create(&Album{Name: album.Name, Path: album.Path}); result.Error != nil {
			logger.Error("Failed to create album:", "result.Error", result.Error)
			return
		}

		// Re-index site photos starting from 0
		for i, photo := range album.SitePhotos {
			photo.Index = i
			if result := db.Create(&Photo{AlbumID: album.ID, Index: photo.Index, Name: photo.Name, Path: photo.Path, Type: photo.Type}); result.Error != nil {
				logger.Error("Failed to create site photo:", "result.Error", result.Error)
				return
			}
		}

		// Re-index original photos starting from 0
		for i, photo := range album.OriginalPhotos {
			photo.Index = i
			if result := db.Create(&Photo{AlbumID: album.ID, Index: photo.Index, Name: photo.Name, Path: photo.Path, Type: photo.Type}); result.Error != nil {
				logger.Error("Failed to create original photo:", "result.Error", result.Error)
				return
			}
		}
	}

	// iterate through all albums, site photos, and original photos and  output the indexes, names, etc... to the terminal

	result := db.Find(&albums)
	if result.Error != nil {

		logger.Error("Error reading albums:", "result.Error", result.Error)
		return
	}

	for _, album := range albums {
		logger.Info("Found album", "album.ID", album.ID, "album.Name", album.Name, "album.Path", album.Path)

		var sitePhotos []Photo
		result := db.Where("album_id = ? AND type = ?", album.ID, "page").Find(&sitePhotos)
		if result.Error != nil {
			logger.Error("Error reading site photos for album:", "album.ID", album.ID, "result.Error", result.Error)
			return
		}

		for _, photo := range sitePhotos {
			logger.Info("Found site photo", "photo.ID", photo.ID, "photo.AlbumID", photo.AlbumID, "photo.Index", photo.Index, "photo.Name", photo.Name, "photo.Path", photo.Path)
		}

		var originalPhotos []Photo
		result = db.Where("album_id = ? AND type = ?", album.ID, "original").Find(&originalPhotos)
		if result.Error != nil {
			logger.Error("Error reading original photos for album:", "album.ID", album.ID, "result.Error", result.Error)
			return
		}

		for _, photo := range originalPhotos {
			logger.Info("Found original photo", "photo.ID", photo.ID, "photo.AlbumID", photo.AlbumID, "photo.Index", photo.Index, "photo.Name", photo.Name, "photo.Path", photo.Path)
		}

	}

	return
}

/*
func initializePhotos() {

}

func initializeUsers() {

}
*/

func main() {

	currentUser, err := user.Current()
	if err != nil {
		log.Fatalf(err.Error())
	}

	logger.Info("Blazemarker initialization server starting", "Name", currentUser.Name)

	migrateArticlesToDB()
	migratePhotosToDB()
	//initializePhotos()
	//initializeUsers()

	logger.Info("Blazemarker initialization server ending", "Name", currentUser.Name)
}

// Path: initialize_db.go
