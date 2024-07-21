package main

import (
	"log"
	"log/slog"
	"os"
	"os/user"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func initializeDB() {
	// Open SQLite database
	db, err := gorm.Open(sqlite.Open("./blazemarker.db"), &gorm.Config{})
	if err != nil {
		log.Fatal(err)
	}

	// Migrate the schema
	db.AutoMigrate(&Article{})
	db.AutoMigrate(&Album{}, &Photo{})
}

func initializeArticles() {

}

func initializePhotos() {

}

func initializeUsers() {

}

func initializeInitSiteLog() {
	f, err := os.OpenFile("../logs/blazemarker_init.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("error opening log file: %v", err)
	}

	init_initsitelogger = slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{AddSource: true, Level: slog.LevelDebug}))
	init_logger.Debug("Logging initialized", "AddSource", "true", "Level", "LevelDebug")

	//slog.SetLogLoggerLevel(slog.LevelDebug)
}

func main() {
	initializeInitSiteLog()

	currentUser, err := user.Current()
	if err != nil {
		log.Fatalf(err.Error())
	}

	initializeDB()
	initializeArticles()
	initializePhotos()
	initializeUsers()

	logger.Info("Blazemarker initialization server starting", "Name", currentUser.Name, "Id")
}
