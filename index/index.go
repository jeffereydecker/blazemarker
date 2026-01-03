package main

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/jeffereydecker/blazemarker/blaze_db"
	"github.com/jeffereydecker/blazemarker/blaze_log"
	"github.com/jeffereydecker/blazemarker/blog_db"
	"github.com/jeffereydecker/blazemarker/gallery_db"
	"github.com/jeffereydecker/blazemarker/user_db"
	"github.com/tg123/go-htpasswd"
)

// Aliases
type Article = blog_db.Article
type Photo = gallery_db.Photo
type Album = gallery_db.Album
type UserProfile = user_db.UserProfile

var logger *slog.Logger = blaze_log.GetLogger()
var db *gorm.DB = blaze_db.GetDB()
var adminUsers map[string]bool

// loadAdminUsers loads the list of admin users from config file
func loadAdminUsers() {
	adminUsers = make(map[string]bool)

	data, err := os.ReadFile("../config/admins.txt")
	if err != nil {
		logger.Error("Failed to load admin users file", "error", err)
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		username := strings.TrimSpace(line)
		if username != "" {
			adminUsers[username] = true
			logger.Info("Loaded admin user", "username", username)
		}
	}
}

// isAdmin checks if a username is an admin
func isAdmin(username string) bool {
	return adminUsers[username]
}

type Blog struct {
	Title       string    `json:"title"`
	Articles    []Article `json:"articles"`
	SearchQuery string    `json:"search_query"`
}

type ArticleWithProfile struct {
	Article
	Profile *UserProfile
}

type Gallery struct {
	Title  string  `json:"title"`
	Albums []Album `json:"albums"`
}

// Helper function to enrich articles with user profiles
func enrichArticlesWithProfiles(articles []Article) []ArticleWithProfile {
	enriched := make([]ArticleWithProfile, len(articles))
	for i, article := range articles {
		profile, _ := user_db.GetUserProfile(db, article.Author)
		enriched[i] = ArticleWithProfile{
			Article: article,
			Profile: profile,
		}
	}
	return enriched
}

// Template function map for user profile lookups
func getTemplateFuncs() template.FuncMap {
	return template.FuncMap{
		"getUserProfile": func(username string) *UserProfile {
			profile, _ := user_db.GetUserProfile(db, username)
			if profile != nil {
				profile.IsAdmin = isAdmin(username)
			}
			return profile
		},
		"upper": strings.ToUpper,
		"slice": func(s string, start, end int) string {
			if start < 0 || start >= len(s) {
				return ""
			}
			if end > len(s) {
				end = len(s)
			}
			return s[start:end]
		},
	}
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

	t := template.New("base.html").Funcs(getTemplateFuncs())
	t, _ = t.ParseFiles("../templates/base.html", "../templates/index.html")
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
	pageData.Title = "Welcome Home"
	pageData.Articles = blog_db.GetIndexArticles(db)

	t := template.New("base.html").Funcs(getTemplateFuncs())
	t, _ = t.ParseFiles("../templates/base.html", "../templates/index.html")
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

func servProfile(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	switch r.Method {
	case http.MethodGet:
		// Display profile
		profile, err := user_db.GetUserProfile(db, username)
		if err != nil {
			logger.Error("Error getting user profile", "error", err)
			http.Error(w, "Error loading profile", http.StatusInternalServerError)
			return
		}
		profile.IsAdmin = isAdmin(username)

		t := template.New("base.html").Funcs(getTemplateFuncs())
		t, _ = t.ParseFiles("../templates/base.html", "../templates/profile.html")
		err = t.Execute(w, profile)
		if err != nil {
			logger.Error(err.Error())
			return
		}

	case http.MethodPost:
		// Update profile
		r.ParseMultipartForm(10 << 20) // 10 MB max

		profile, err := user_db.GetUserProfile(db, username)
		if err != nil {
			logger.Error("Error getting user profile", "error", err)
			http.Error(w, "Error loading profile", http.StatusInternalServerError)
			return
		}

		// Update fields
		profile.Handle = r.FormValue("handle")
		profile.Email = r.FormValue("email")
		profile.Phone = r.FormValue("phone")

		// Handle avatar upload
		file, header, err := r.FormFile("avatar")
		if err == nil {
			defer file.Close()

			// Create avatars directory if it doesn't exist
			avatarsDir := "../photos/avatars"
			os.MkdirAll(avatarsDir, os.ModePerm)

			// Save file with username as filename
			ext := filepath.Ext(header.Filename)
			filename := username + ext
			avatarPath := filepath.Join(avatarsDir, filename)

			dst, err := os.Create(avatarPath)
			if err != nil {
				logger.Error("Error creating avatar file", "error", err)
				http.Error(w, "Error saving avatar", http.StatusInternalServerError)
				return
			}
			defer dst.Close()

			if _, err := io.Copy(dst, file); err != nil {
				logger.Error("Error saving avatar", "error", err)
				http.Error(w, "Error saving avatar", http.StatusInternalServerError)
				return
			}

			profile.AvatarPath = "/photos/avatars/" + filename
		}

		// Save profile
		err = user_db.UpdateUserProfile(db, profile)
		if err != nil {
			logger.Error("Error updating profile", "error", err)
			http.Error(w, "Error saving profile", http.StatusInternalServerError)
			return
		}

		// Redirect back to profile
		http.Redirect(w, r, "/profile", http.StatusSeeOther)
	}
}

func servChangePassword(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	type PageData struct {
		Error   string
		Success bool
	}

	switch r.Method {
	case http.MethodGet:
		// Display change password form
		t := template.New("base.html").Funcs(getTemplateFuncs())
		t, _ = t.ParseFiles("../templates/base.html", "../templates/changepassword.html")
		err := t.Execute(w, PageData{})
		if err != nil {
			logger.Error(err.Error())
			return
		}

	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			logger.Error("Form parsing error")
			http.Error(w, "Form parsing error", http.StatusBadRequest)
			return
		}

		currentPassword := r.FormValue("current_password")
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")

		// Verify current password
		myauth, err := htpasswd.New("../blaze_auth/.htpasswd", htpasswd.DefaultSystems, nil)
		if err != nil {
			logger.Error("Error loading htpasswd", "error", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		if !myauth.Match(username, currentPassword) {
			t := template.New("base.html").Funcs(getTemplateFuncs())
			t, _ = t.ParseFiles("../templates/base.html", "../templates/changepassword.html")
			t.Execute(w, PageData{Error: "Current password is incorrect"})
			return
		}

		// Validate new passwords match
		if newPassword != confirmPassword {
			t := template.New("base.html").Funcs(getTemplateFuncs())
			t, _ = t.ParseFiles("../templates/base.html", "../templates/changepassword.html")
			t.Execute(w, PageData{Error: "New passwords do not match"})
			return
		}

		// Validate password length
		if len(newPassword) < 6 {
			t := template.New("base.html").Funcs(getTemplateFuncs())
			t, _ = t.ParseFiles("../templates/base.html", "../templates/changepassword.html")
			t.Execute(w, PageData{Error: "Password must be at least 6 characters"})
			return
		}

		// Hash new password using bcrypt (same as htpasswd)
		hashedBytes, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			logger.Error("Error hashing password", "error", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}
		hashedPassword := string(hashedBytes)

		// Read htpasswd file
		htpasswdPath := "../blaze_auth/.htpasswd"
		data, err := os.ReadFile(htpasswdPath)
		if err != nil {
			logger.Error("Error reading htpasswd file", "error", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		// Update user's line
		lines := strings.Split(string(data), "\n")
		var newLines []string
		updated := false

		for _, line := range lines {
			if strings.HasPrefix(line, username+":") {
				newLines = append(newLines, username+":"+hashedPassword)
				updated = true
			} else if line != "" {
				newLines = append(newLines, line)
			}
		}

		if !updated {
			logger.Error("User not found in htpasswd", "username", username)
			http.Error(w, "User not found", http.StatusInternalServerError)
			return
		}

		// Write back to file
		newContent := strings.Join(newLines, "\n") + "\n"
		err = os.WriteFile(htpasswdPath, []byte(newContent), 0600)
		if err != nil {
			logger.Error("Error writing htpasswd file", "error", err)
			http.Error(w, "Server error", http.StatusInternalServerError)
			return
		}

		logger.Info("Password changed successfully", "username", username)

		// Show success message
		t := template.New("base.html").Funcs(getTemplateFuncs())
		t, _ = t.ParseFiles("../templates/base.html", "../templates/changepassword.html")
		t.Execute(w, PageData{Success: true})
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
		// Get user profile
		profile, _ := user_db.GetUserProfile(db, username)
		if profile != nil {
			profile.IsAdmin = isAdmin(username)
			logger.Debug("servArticle() - User profile loaded", "username", username, "isAdmin", profile.IsAdmin)
		}

		// Check if updating an existing article
		articleIDStr := r.URL.Query().Get("id")
		if len(articleIDStr) > 0 {
			// Parse article ID and load existing article
			var articleID uint
			if _, err := fmt.Sscanf(articleIDStr, "%d", &articleID); err != nil {
				logger.Error("Invalid article ID:", "articleIDStr", articleIDStr, "error", err)
				http.Error(w, "Invalid article ID", http.StatusBadRequest)
				return
			}

			article, err := blog_db.GetArticleByID(db, articleID)
			if err != nil {
				logger.Error("Article not found:", "articleID", articleID)
				http.Error(w, "Article not found", http.StatusNotFound)
				return
			}

			logger.Debug("servArticle()[GET] - Edit existing article", "articleID", articleID, "title", article.Title)

			pageData := ArticleWithProfile{
				Article: article,
				Profile: profile,
			}

			t, _ := template.ParseFiles("../templates/base.html", "../templates/newarticle.html")
			err = t.Execute(w, pageData)
			if err != nil {
				logger.Error(err.Error())
				return
			}
		} else {
			// Create new article
			pageData := ArticleWithProfile{
				Article: Article{Title: "New Article"},
				Profile: profile,
			}

			logger.Debug("servArticle()[GET] - Create new article")

			t, _ := template.ParseFiles("../templates/base.html", "../templates/newarticle.html")
			err := t.Execute(w, pageData)

			if err != nil {
				logger.Error(err.Error())
				return
			}
		}
	case http.MethodPost:
		logger.Debug("servArticle()[POST]")

		if err := r.ParseForm(); err != nil {
			logger.Error("Form parsing error")
			http.Error(w, "Form parsing error", http.StatusBadRequest)
			return
		}

		// Check if this is an update (article ID is provided)
		articleIDStr := r.FormValue("id")
		if len(articleIDStr) > 0 {
			// Update existing article
			var articleID uint
			if _, err := fmt.Sscanf(articleIDStr, "%d", &articleID); err != nil {
				logger.Error("Invalid article ID:", "articleIDStr", articleIDStr, "error", err)
				http.Error(w, "Invalid article ID", http.StatusBadRequest)
				return
			}

			article, err := blog_db.GetArticleByID(db, articleID)
			if err != nil {
				logger.Error("Article not found:", "articleID", articleID)
				http.Error(w, "Article not found", http.StatusNotFound)
				return
			}

			// Update fields
			article.Title = r.FormValue("title")
			article.Content = template.HTML(r.FormValue("content"))
			article.Author = username
			article.IsNow = r.FormValue("is_now") == "on"
			article.IsPrivate = r.FormValue("is_private") == "on"
			article.IsIndex = r.FormValue("is_index") == "on"

			if ok := blog_db.UpdateArticle(db, article); !ok {
				logger.Error("Failed to update article", "articleID", articleID, "title", article.Title)
				http.Error(w, "Failed to update article", http.StatusInternalServerError)
				return
			}

			logger.Info("Article updated successfully", "articleID", articleID, "title", article.Title)
		} else {
			// Create new article
			var article Article
			article.Title = r.FormValue("title")
			article.Content = template.HTML(r.FormValue("content"))
			article.Date = time.Now().Format("2006-01-02")
			article.Author = username
			article.IsNow = r.FormValue("is_now") == "on"
			article.IsPrivate = r.FormValue("is_private") == "on"
			article.IsIndex = r.FormValue("is_index") == "on"

			if ok := blog_db.SaveArticle(db, article); !ok {
				logger.Error("Failed to save article", "title", article.Title, "author", article.Author)
				http.Error(w, "Failed to save article", http.StatusInternalServerError)
				return
			}

			logger.Info("New article created successfully", "title", article.Title, "author", article.Author)
		}

		http.Redirect(w, r, "/articles", http.StatusFound)
	default:
		logger.Info("Method not allowed", "r.Method", r.Method)
	}

}

func servDeleteArticle(w http.ResponseWriter, r *http.Request) {
	var ok bool

	if ok, _ = basicAuth(w, r); !ok {
		logger.Info("Failed baseAuth attempt")
		return
	}

	if r.Method != http.MethodPost {
		logger.Info("Method not allowed for delete", "r.Method", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		logger.Error("Form parsing error")
		http.Error(w, "Form parsing error", http.StatusBadRequest)
		return
	}

	// Extract article ID from URL path (e.g., /article/123)
	path := strings.TrimPrefix(r.URL.Path, "/article/")
	if len(path) == 0 {
		logger.Error("Missing article ID in request")
		http.Error(w, "Missing article ID", http.StatusBadRequest)
		return
	}

	var articleID uint
	if _, err := fmt.Sscanf(path, "%d", &articleID); err != nil {
		logger.Error("Invalid article ID:", "articleID", path, "error", err)
		http.Error(w, "Invalid article ID", http.StatusBadRequest)
		return
	}

	if ok := blog_db.DeleteArticle(db, articleID); !ok {
		logger.Error("Failed to delete article", "articleID", articleID)
		http.Error(w, "Failed to delete article", http.StatusInternalServerError)
		return
	}

	logger.Info("Article deleted successfully", "articleID", articleID)
	http.Redirect(w, r, "/articles", http.StatusFound)
}

func servArticleView(w http.ResponseWriter, r *http.Request) {
	if ok, _ := basicAuth(w, r); !ok {
		logger.Info("Failed baseAuth attempt")
		return
	}

	// Extract article ID from URL path (e.g., /article/view/123)
	path := strings.TrimPrefix(r.URL.Path, "/article/view/")
	if len(path) == 0 {
		logger.Error("Missing article ID in request")
		http.Error(w, "Missing article ID", http.StatusBadRequest)
		return
	}

	var articleID uint
	if _, err := fmt.Sscanf(path, "%d", &articleID); err != nil {
		logger.Error("Invalid article ID:", "articleID", path, "error", err)
		http.Error(w, "Invalid article ID", http.StatusBadRequest)
		return
	}

	article, err := blog_db.GetArticleByID(db, articleID)
	if err != nil {
		logger.Error("Article not found:", "articleID", articleID)
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	logger.Debug("servArticleView()", "articleID", articleID, "title", article.Title)

	t := template.New("base.html").Funcs(getTemplateFuncs())
	t, _ = t.ParseFiles("../templates/base.html", "../templates/article_view.html")
	err = t.Execute(w, article)

	if err != nil {
		logger.Error(err.Error())
		return
	}
}

func servArticles(w http.ResponseWriter, r *http.Request) {
	if ok, _ := basicAuth(w, r); !ok {
		logger.Info("Failed baseAuth attempt")
		return
	}

	pageData := new(Blog)

	// Check if there's a search query
	searchQuery := r.URL.Query().Get("q")

	if len(searchQuery) > 0 {
		// Perform search
		pageData.Title = "Search Results for \"" + searchQuery + "\""
		pageData.SearchQuery = searchQuery
		pageData.Articles = blog_db.SearchArticles(db, searchQuery)
		logger.Debug("servArticles() - Search", "query", searchQuery, "results", len(pageData.Articles))
	} else {
		// Show all articles
		pageData.Title = "Decker News"
		pageData.Articles = blog_db.GetAllArticles(db)
		logger.Debug("servArticles()")
	}

	blog_db.SortByDate(pageData.Articles)

	t := template.New("base.html").Funcs(getTemplateFuncs())
	t, _ = t.ParseFiles("../templates/base.html", "../templates/articles.html")
	err := t.Execute(w, pageData)

	if err != nil {
		logger.Error(err.Error())
		return
	}
}

func servPrivateArticles(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed baseAuth attempt")
		return
	}

	pageData := new(Blog)
	pageData.Title = "Private Journal - " + username

	logger.Debug("servPrivateArticles()", "username", username)

	// Get only private articles for this user
	pageData.Articles = blog_db.GetPrivateArticles(db, username)

	blog_db.SortByDate(pageData.Articles)

	t := template.New("base.html").Funcs(getTemplateFuncs())
	t, _ = t.ParseFiles("../templates/base.html", "../templates/articles.html")
	err := t.Execute(w, pageData)

	if err != nil {
		logger.Error(err.Error())
		return
	}
}

func servMyArticles(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed baseAuth attempt")
		return
	}

	pageData := new(Blog)
	pageData.Title = "My Articles - " + username

	logger.Debug("servMyArticles()", "username", username)

	// Get only non-private articles for this user
	pageData.Articles = blog_db.GetMyArticles(db, username)

	blog_db.SortByDate(pageData.Articles)

	t := template.New("base.html").Funcs(getTemplateFuncs())
	t, _ = t.ParseFiles("../templates/base.html", "../templates/articles.html")
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

	// Load admin users from config
	loadAdminUsers()

	// TODO: Test general access to file system
	// TODO: Look for ways to lock down to specific directories
	http.Handle("/photos/galleries/", http.StripPrefix("/photos/galleries/", http.FileServer(http.Dir("../photos/galleries"))))
	http.Handle("/photos/avatars/", http.StripPrefix("/photos/avatars/", http.FileServer(http.Dir("../photos/avatars"))))
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
	http.HandleFunc("/profile", servProfile)
	http.HandleFunc("/changepassword", servChangePassword)
	http.HandleFunc("/articles", servArticles)
	http.HandleFunc("/myarticles", servMyArticles)
	http.HandleFunc("/private", servPrivateArticles)

	// Article handler with custom routing for view, edit, and delete
	http.HandleFunc("/article", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/article/view/") {
			// View single article
			servArticleView(w, r)
		} else if strings.HasPrefix(r.URL.Path, "/article/") && r.Method == http.MethodPost {
			// This is a DELETE request
			servDeleteArticle(w, r)
		} else {
			// This is a GET or POST for creating/editing
			servArticle(w, r)
		}
	})
	http.HandleFunc("/article/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/article/view/") {
			// View single article
			servArticleView(w, r)
		} else if r.Method == http.MethodPost {
			servDeleteArticle(w, r)
		} else {
			http.NotFound(w, r)
		}
	})

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
