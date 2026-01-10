module github.com/jeffereydecker/blazemarker/chat_db

go 1.22.5

require (
	github.com/jeffereydecker/blazemarker/blaze_log v0.0.0
	gorm.io/gorm v1.25.12
)

replace github.com/jeffereydecker/blazemarker/blaze_log => ../blaze_log
