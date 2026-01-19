package blaze_log

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

var (
	logger *slog.Logger = nil
	once   sync.Once
)

func getBasePath() string {
	exePath, err := os.Executable()
	if err != nil {
		panic(err)
	}
	return filepath.Dir(exePath)
}

func InitializeLogOnce() {

	if logger == nil {
		logPath := filepath.Join(getBasePath(), "../logs", "blazemarker.log")
		f, err := os.OpenFile(logPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal("error opening log file: ", err.Error())
		}

		logger = slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{AddSource: true, Level: slog.LevelDebug}))
		logger.Debug("Logging initialized", "AddSource", "true", "Level", "LevelDebug")

		//slog.SetLogLoggerLevel(slog.LevelDebug)
	}
}

func GetLogger() *slog.Logger {
	once.Do(InitializeLogOnce)

	return logger
}
