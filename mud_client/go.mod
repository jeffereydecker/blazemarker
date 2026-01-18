module github.com/jeffereydecker/blazemarker/mud_client

go 1.23.0

require (
	github.com/chromedp/chromedp v0.11.2
	github.com/jeffereydecker/blazemarker/blaze_log v0.0.0
	github.com/jeffereydecker/blazemarker/chat_db v0.0.0
)

require (
	github.com/chromedp/cdproto v0.0.0-20241022234722-4d5d5faf59fb // indirect
	github.com/chromedp/sysutil v1.1.0 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.4.0 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	golang.org/x/sys v0.26.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	gorm.io/gorm v1.25.12 // indirect
)

replace github.com/jeffereydecker/blazemarker/chat_db => ../chat_db

replace github.com/jeffereydecker/blazemarker/blaze_log => ../blaze_log
