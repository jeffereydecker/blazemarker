package blaze_log

import (
	"log"
	"log/slog"
	"os"
	"sync"
)

var (
	logger *slog.Logger = nil
	once   sync.Once
)

func InitializeLogOnce() {

	if logger == nil {
		f, err := os.OpenFile("logs/blazemarker.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
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
