module github.com/jeffereydecker/blazemarker/push_db

go 1.23.0

require (
	github.com/jeffereydecker/blazemarker/blaze_log v0.0.0
	gorm.io/gorm v1.25.12
)

replace github.com/jeffereydecker/blazemarker/blaze_log => ../blaze_log
