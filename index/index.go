package main

import (
	"fmt"
	"html/template"
	"log"
	"log/slog"
	"mime"
	"net/http"
	"os/user"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/jeffereydecker/blazemarker/blaze_db"
	"github.com/jeffereydecker/blazemarker/blaze_log"
	"github.com/jeffereydecker/blazemarker/blog_db"
	"github.com/jeffereydecker/blazemarker/gallery_db"
	"github.com/tg123/go-htpasswd"
)

// Aliases
type Article = blog_db.Article
type Photo = gallery_db.Photo
type Album = gallery_db.Album

var logger *slog.Logger = blaze_log.GetLogger()
var db *gorm.DB = blaze_db.GetDB()

type Blog struct {
	Title    string    `json:"title"`
	Articles []Article `json:"articles"`
}

type Gallery struct {
	Title  string  `json:"title"`
	Albums []Album `json:"albums"`
}

func servNow(w http.ResponseWriter, r *http.Request) {
	// The root handler "/" matches every path that wasn't match by other
	// matchers, so we have to further filter it here. Only accept actual root
	// paths.

	//if path := strings.Trim(r.URL.Path, "/index"); len(path) > 0 {
	//	fmt.Println("/index NOT FOUND r.URL.Path" + r.URL.Path)
	//
	//	http.NotFound(w, r)
	//	return
	//}

	logger.Debug("servNow()")

	pageData := new(Blog)
	pageData.Title = "What I'm Doing Now"
	pageData.Articles = blog_db.GetNowArticles(db)

	t, _ := template.ParseFiles("../templates/base.html", "../templates/index.html")
	err := t.Execute(w, pageData)

	if err != nil {
		logger.Error(err.Error())
		return
	}
}

func servIndex(w http.ResponseWriter, r *http.Request) {
	// The root handler "/" matches every path that wasn't match by other
	// matchers, so we have to further filter it here. Only accept actual root
	// paths.

	if path := strings.Trim(r.URL.Path, "/index"); len(path) > 0 {
		fmt.Println("/index NOT FOUND r.URL.Path" + r.URL.Path)

		http.NotFound(w, r)
		return
	}

	logger.Debug("servIndex()")

	pageData := new(Blog)
	pageData.Title = "Jefferey Decker"
	pageData.Articles = blog_db.GetIndexArticles(db)

	t, _ := template.ParseFiles("../templates/base.html", "../templates/index.html")
	err := t.Execute(w, pageData)

	if err != nil {
		logger.Error(err.Error())
		return
	}
}

func basicAuth(w http.ResponseWriter, r *http.Request) (bool, string) {
	username, password, ok := r.BasicAuth()

	if !ok {
		w.Header().Add("WWW-Authenticate", `Basic realm="Give username and password"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "No basic auth present"}`))

		logger.Error("No basic auth present")
		return ok, ""
	}

	myauth, err := htpasswd.New("../blaze_auth/.htpasswd", htpasswd.DefaultSystems, nil)
	if err != nil {
		logger.Error(err.Error())
		return false, ""
	}

	if ok = myauth.Match(username, password); !ok {
		w.Header().Add("WWW-Authenticate", `Basic realm="Give username and password"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message": "No basic auth present"}`))

		logger.Info("Blazemarker, basicAuth(), Unauthorized", "username", username)
		return ok, username
	}

	logger.Info("Blazemarker, basicAuth(), Authorized", "username", username, "password", password)
	return true, username
}

//TODO:
// Paging: Start: 1, Num: 4
//         End: 75 (Num Pages/4), Num: 4
//         Next: Current + 1 if Current < Max; Otherwise disable
//         Previous: Current -1 if Current > Start; Otherwise disable
//         Middle: 300/4 = 75, 75/2 = 37
// Assuming 300
// Num Pages: 300/4 = 75
//  From Page 1: DISABLE(<<1), DISABLE (<), 2>, 37> 75>>
//  From Page 2: <<1 <1, 3>, 75>>
//  From Page 37: <<1, <36, 38>, 75>>
//  Create an input to go direclty to page

func servGallery(w http.ResponseWriter, r *http.Request) {
	if ok, _ := basicAuth(w, r); !ok {
		logger.Info("Failed baseAuth attempt")
		return
	}

	pageData := new(Gallery)
	pageData.Title = "Decker Photo Albums"
	pageData.Albums = gallery_db.GetAllAlbums(db)

	t, _ := template.ParseFiles("../templates/base.html", "../templates/gallery.html")
	err := t.Execute(w, pageData)

	if err != nil {
		logger.Error(err.Error())
		return
	}
}

func servAlbum(w http.ResponseWriter, r *http.Request) {

	if ok, _ := basicAuth(w, r); !ok {
		logger.Info("Failed baseAuth attempt")
		return
	}

	pageData := new(Album)
	pageData.Name = r.URL.Query().Get("name")
	if len(pageData.Name) == 0 {
		logger.Warn("HTTP Request Filter Not Available: name")
		return
	}
	pageData.SitePhotos, pageData.OriginalPhotos = gallery_db.GetAlbumPhotos(db, pageData.Name)

	logger.Debug("servAlbum()", "r.URL.Path", r.URL.Path, "pageData.Name", pageData.Name, "pageData.Path", pageData.Path)

	t, _ := template.ParseFiles("../templates/base.html", "../templates/album.html")
	err := t.Execute(w, pageData)

	if err != nil {
		logger.Error(err.Error())
		return
	}
}

func servArticle(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed baseAuth attempt")
		return
	}
	switch r.Method {
	case http.MethodGet:
		pageData := new(Article)
		pageData.Title = "New Article"

		logger.Debug("servArticle()[GET]")

		t, _ := template.ParseFiles("../templates/base.html", "../templates/newarticle.html")
		err := t.Execute(w, pageData)

		if err != nil {
			logger.Error(err.Error())
			return
		}
	case http.MethodPost:
		logger.Debug("servArticle()[POST]")

		if err := r.ParseForm(); err != nil {
			logger.Error("Form parsing error")
			http.Error(w, "Form parsing error", http.StatusBadRequest)
			return
		}
		var article Article
		article.Title = r.FormValue("title")
		article.Content = template.HTML(r.FormValue("content"))
		article.Date = time.Now().Format("2006-01-02")
		article.Author = username
		article.IsNow = r.FormValue("is_now") == "on"

		if ok := blog_db.SaveArticle(db, article); !ok {
			logger.Error("Failed to save article", "article.Title", article.Title, "article.Author", article.Title)

		}

		http.Redirect(w, r, "/articles", http.StatusFound)
	default:
		logger.Info("Method not allowed", "r.Method", r.Method)
	}

}

func servArticles(w http.ResponseWriter, r *http.Request) {
	if ok, _ := basicAuth(w, r); !ok {
		logger.Info("Failed baseAuth attempt")
		return
	}

	pageData := new(Blog)
	pageData.Title = "Decker News"

	logger.Debug("servArticles()")

	pageData.Articles = blog_db.GetAllArticles(db)

	blog_db.SortByDate(pageData.Articles)

	t, _ := template.ParseFiles("../templates/base.html", "../templates/articles.html")
	err := t.Execute(w, pageData)

	if err != nil {
		logger.Error(err.Error())
		return
	}
}

func main() {

	currentUser, err := user.Current()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// TODO: Test general access to file system
	// TODO: Look for ways to lock down to specific directories
	http.Handle("/photos/galleries/", http.StripPrefix("/photos/galleries/", http.FileServer(http.Dir("../photos/galleries"))))
	http.Handle("/bootstrap-5.3.0-dist/", http.StripPrefix("/bootstrap-5.3.0-dist/", http.FileServer(http.Dir("../bootstrap-5.3.0-dist"))))
	http.Handle("/tinymce/", http.StripPrefix("/tinymce/", http.FileServer(http.Dir("../tinymce"))))
	http.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("../css"))))

	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../static/favicon.ico")
	})

	http.HandleFunc("/android-chrome-192x192.png", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../static/android-chrome-192x192.png")
	})

	http.HandleFunc("/android-chrome-512x512.png", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../static/android-chrome-512x512.png")
	})

	http.HandleFunc("/apple-touch-icon.png", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../static/apple-touch-icon.png")
	})

	http.HandleFunc("/favicon-16x16.png", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../static/favicon-16x16.png")
	})

	http.HandleFunc("/favicon-32x32.png", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../static/favicon-32x32.png")
	})

	// TODO: Update /index to show photos, videos and blog and maybe an random photo, video or blog?  Or an about page
	http.HandleFunc("/index", servIndex)
	http.HandleFunc("/", servIndex)
	http.HandleFunc("/now", servNow)
	http.HandleFunc("/articles", servArticles)
	http.HandleFunc("/article", servArticle)

	// TODO: upate gallery to have paging, update color scheme
	http.HandleFunc("/gallery", servGallery)
	// TODO: code /album functionality. For example, carousel?
	http.HandleFunc("/album", servAlbum)

	mime.AddExtensionType(".css", "text/css")
	mime.AddExtensionType(".js", "application/javascript")
	mime.AddExtensionType(".jpeg", "image/jpeg")
	mime.AddExtensionType(".jpg", "image/jpeg")
	mime.AddExtensionType(".gif", "image/gif")
	mime.AddExtensionType(".png", "image/png")
	mime.AddExtensionType(".svg", "image/svg+xml")
	mime.AddExtensionType(".svgz", "image/svg+xml")

	logger.Info("Blazemarker server starting", "Name", currentUser.Name, "Id", currentUser.Uid, "Port", "3000")
	http.ListenAndServe(":3000", nil)

}
