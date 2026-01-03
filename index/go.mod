module github.com/jeffereydecker/blazemarker/index

go 1.23.0

require (
	github.com/jeffereydecker/blazemarker/blaze_db v0.0.0-00010101000000-000000000000
	github.com/jeffereydecker/blazemarker/blaze_log v0.0.0
	github.com/jeffereydecker/blazemarker/blog_db v0.0.0-20240721140226-fd4ad63d62d4
	github.com/jeffereydecker/blazemarker/gallery_db v0.0.0-20240721140226-fd4ad63d62d4
	github.com/jeffereydecker/blazemarker/user_db v0.0.0-00010101000000-000000000000
	github.com/tg123/go-htpasswd v1.2.2
	gorm.io/gorm v1.25.11
)

require (
	github.com/GehirnInc/crypt v0.0.0-20200316065508-bb7000b8a962 // indirect
	github.com/disintegration/imaging v1.6.2 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	golang.org/x/crypto v0.17.0 // indirect
	golang.org/x/image v0.18.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	gorm.io/driver/sqlite v1.5.6 // indirect
)

replace (
	github.com/jeffereydecker/blazemarker/blaze_db => ../blaze_db
	github.com/jeffereydecker/blazemarker/blaze_log => ../blaze_log
	github.com/jeffereydecker/blazemarker/blog_db => ../blog_db
	github.com/jeffereydecker/blazemarker/gallery_db => ../gallery_db
	github.com/jeffereydecker/blazemarker/user_db => ../user_db
)
