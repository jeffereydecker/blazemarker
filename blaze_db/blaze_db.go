package blaze_db

import (
	"log"
	"sync"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/jeffereydecker/blazemarker/blog_db"
	"github.com/jeffereydecker/blazemarker/gallery_db"
)

var (
	db   *gorm.DB = nil
	once sync.Once
)

// Aliases
type Article = blog_db.Article
type Photo = gallery_db.Photo
type Album = gallery_db.Album

func initializeDBOnce() {
	// Open SQLite database
	var err error
	if db == nil {
		db, err = gorm.Open(sqlite.Open("../data/blazemarker.db"), &gorm.Config{})
		if err != nil {
			log.Fatal(err)
		}

		// Migrate the schema
		db.AutoMigrate(&Article{})
		db.AutoMigrate(&Album{}, &Photo{})
	}
}

func GetDB() *gorm.DB {
	once.Do(initializeDBOnce)

	return db
}
