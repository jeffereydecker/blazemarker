package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
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
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/jeffereydecker/blazemarker/blaze_db"
	"github.com/jeffereydecker/blazemarker/blaze_email"
	"github.com/jeffereydecker/blazemarker/blaze_log"
	"github.com/jeffereydecker/blazemarker/blog_db"
	"github.com/jeffereydecker/blazemarker/calendar_db"
	"github.com/jeffereydecker/blazemarker/chat_db"
	"github.com/jeffereydecker/blazemarker/gallery_db"
	"github.com/jeffereydecker/blazemarker/push_db"
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
var calendarConfig calendar_db.CalendarConfig

// Session management
type Session struct {
	Username  string
	ExpiresAt time.Time
}

var (
	sessions      = make(map[string]*Session)
	sessionsMutex sync.RWMutex
	sessionTTL    = 7 * 24 * time.Hour // 7 days
)

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

// loadCalendarConfig loads CalDAV configuration from config file or environment
func loadCalendarConfig() {
	// Try environment variables first (more secure)
	serverURL := os.Getenv("CALDAV_SERVER_URL")
	username := os.Getenv("CALDAV_USERNAME")
	password := os.Getenv("CALDAV_PASSWORD")
	calendar := os.Getenv("CALDAV_CALENDAR")

	// Fall back to config file if env vars not set
	if serverURL == "" || username == "" || password == "" {
		data, err := os.ReadFile("../config/caldav.conf")
		if err != nil {
			logger.Error("Failed to load CalDAV config file", "error", err)
			return
		}

		lines := strings.Split(string(data), "\n")
		config := make(map[string]string)

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				config[parts[0]] = parts[1]
			}
		}

		// Use config file values if env vars weren't set
		if serverURL == "" {
			serverURL = config["CALDAV_SERVER_URL"]
		}
		if username == "" {
			username = config["CALDAV_USERNAME"]
		}
		if password == "" {
			password = config["CALDAV_PASSWORD"]
		}
		if calendar == "" {
			calendar = config["CALDAV_CALENDAR"]
		}
	}

	calendarConfig = calendar_db.CalendarConfig{
		ServerURL: serverURL,
		Username:  username,
		Password:  password,
		Calendar:  calendar,
	}

	logger.Info("Loaded CalDAV config", "server", calendarConfig.ServerURL, "username", calendarConfig.Username)
}

// isAdmin checks if a username is an admin
func isAdmin(username string) bool {
	return adminUsers[username]
}

// generateSessionToken generates a random session token
func generateSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// createSession creates a new session for the user
func createSession(username string) (string, error) {
	token, err := generateSessionToken()
	if err != nil {
		return "", err
	}

	sessionsMutex.Lock()
	defer sessionsMutex.Unlock()

	sessions[token] = &Session{
		Username:  username,
		ExpiresAt: time.Now().Add(sessionTTL),
	}

	return token, nil
}

// getSession retrieves a session by token
func getSession(token string) (*Session, bool) {
	sessionsMutex.RLock()
	defer sessionsMutex.RUnlock()

	session, exists := sessions[token]
	if !exists || time.Now().After(session.ExpiresAt) {
		return nil, false
	}

	return session, true
}

// cleanupExpiredSessions periodically removes expired sessions
func cleanupExpiredSessions() {
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		for range ticker.C {
			sessionsMutex.Lock()
			now := time.Now()
			for token, session := range sessions {
				if now.After(session.ExpiresAt) {
					delete(sessions, token)
				}
			}
			sessionsMutex.Unlock()
		}
	}()
}

type Blog struct {
	Title       string               `json:"title"`
	Articles    []ArticleWithProfile `json:"articles"`
	SearchQuery string               `json:"search_query"`
	TagQuery    string               `json:"tag_query"`
}

type ArticleWithProfile struct {
	Article       Article
	Profile       *UserProfile
	AvailableTags []string
	Reactions     map[string][]string
	UserReactions []string
	Comments      []Comment
}

type Comment struct {
	ID        uint
	ArticleID uint
	Username  string
	Content   string
	CreatedAt string
}

type Gallery struct {
	Title  string  `json:"title"`
	Albums []Album `json:"albums"`
}

// Helper function to enrich articles with user profiles and reactions
func enrichArticlesWithProfiles(articles []Article) []ArticleWithProfile {
	enriched := make([]ArticleWithProfile, len(articles))
	for i, article := range articles {
		profile, _ := user_db.GetUserProfile(db, article.Author)
		reactions := blog_db.GetReactions(db, article.ID)

		// Get comments for this article
		dbComments := blog_db.GetComments(db, article.ID)
		comments := make([]Comment, len(dbComments))
		for j, c := range dbComments {
			comments[j] = Comment{
				ID:        c.ID,
				ArticleID: c.ArticleID,
				Username:  c.Username,
				Content:   c.Content,
				CreatedAt: c.CreatedAt.Format("2006-01-02 15:04"),
			}
		}

		enriched[i] = ArticleWithProfile{
			Article:   article,
			Profile:   profile,
			Reactions: reactions,
			Comments:  comments,
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
		"safeHTML": func(s interface{}) template.HTML {
			switch v := s.(type) {
			case template.HTML:
				return v
			case string:
				return template.HTML(v)
			default:
				return template.HTML("")
			}
		},
		"splitTags": func(tags string) []string {
			if tags == "" {
				return []string{}
			}
			tagList := strings.Split(tags, ",")
			result := []string{}
			for _, tag := range tagList {
				trimmed := strings.TrimSpace(tag)
				if trimmed != "" {
					result = append(result, trimmed)
				}
			}
			return result
		},
		"joinUsers": func(users []string) string {
			return strings.Join(users, ", ")
		},
		"getCommenters": func(comments []Comment) string {
			if len(comments) == 0 {
				return ""
			}
			// Get unique commenters
			seenUsers := make(map[string]bool)
			var users []string
			for _, comment := range comments {
				if !seenUsers[comment.Username] {
					seenUsers[comment.Username] = true
					users = append(users, comment.Username)
				}
			}
			return strings.Join(users, ", ")
		},
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"mul": func(a, b int) int {
			return a * b
		},
		"div": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
		"iterate": func(count int) []int {
			result := make([]int, count)
			for i := 0; i < count; i++ {
				result[i] = i
			}
			return result
		},
		"formatDate": func(t *time.Time) string {
			if t == nil {
				return "Never"
			}
			return t.Format("Jan 2, 2006 3:04 PM")
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
	articles := blog_db.GetNowArticles(db)
	pageData.Articles = enrichArticlesWithProfiles(articles)

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
	articles := blog_db.GetIndexArticles(db)
	pageData.Articles = enrichArticlesWithProfiles(articles)

	t := template.New("base.html").Funcs(getTemplateFuncs())
	t, _ = t.ParseFiles("../templates/base.html", "../templates/index.html")
	err := t.Execute(w, pageData)

	if err != nil {
		logger.Error(err.Error())
		return
	}
}

func basicAuth(w http.ResponseWriter, r *http.Request) (bool, string) {
	// First, check for session cookie
	if cookie, err := r.Cookie("session_token"); err == nil {
		if session, valid := getSession(cookie.Value); valid {
			// Extend session on each request
			sessionsMutex.Lock()
			session.ExpiresAt = time.Now().Add(sessionTTL)
			sessionsMutex.Unlock()

			// Update user's last seen timestamp
			if err := user_db.UpdateLastSeen(db, session.Username); err != nil {
				logger.Error("Failed to update last_seen", "username", session.Username, "error", err)
			}

			return true, session.Username
		}
	}

	// Fall back to Basic Auth
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

	// Create session and set cookie
	token, err := createSession(username)
	if err != nil {
		logger.Error("Failed to create session", "error", err)
	} else {
		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    token,
			Path:     "/",
			MaxAge:   int(sessionTTL.Seconds()),
			HttpOnly: true,
			Secure:   false, // Set to true if using HTTPS
			SameSite: http.SameSiteLaxMode,
		})
	}

	// Update user's last seen timestamp
	if err := user_db.UpdateLastSeen(db, username); err != nil {
		logger.Error("Failed to update last_seen", "username", username, "error", err)
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

func servChat(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	profile, err := user_db.GetUserProfile(db, username)
	if err != nil {
		logger.Error("Error getting user profile", "error", err)
		http.Error(w, "Error loading profile", http.StatusInternalServerError)
		return
	}
	profile.IsAdmin = isAdmin(username)

	t := template.New("base.html").Funcs(getTemplateFuncs())
	t, _ = t.ParseFiles("../templates/base.html", "../templates/chat.html")
	err = t.Execute(w, profile)
	if err != nil {
		logger.Error(err.Error())
		return
	}
}

func servCalendar(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	// Parse month parameter (format: YYYY-MM)
	monthParam := r.URL.Query().Get("month")
	var targetDate time.Time
	if monthParam != "" {
		parsed, err := time.Parse("2006-01", monthParam)
		if err == nil {
			targetDate = parsed
		} else {
			targetDate = time.Now()
		}
	} else {
		targetDate = time.Now()
	}

	// Get first and last day of the month
	year, month, _ := targetDate.Date()
	firstDay := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	lastDay := firstDay.AddDate(0, 1, -1)

	// Extend range to include days from previous/next month to fill calendar grid
	startDate := firstDay
	for startDate.Weekday() != time.Sunday {
		startDate = startDate.AddDate(0, 0, -1)
	}
	endDate := lastDay
	for endDate.Weekday() != time.Saturday {
		endDate = endDate.AddDate(0, 0, 1)
	}

	// Fetch events from CalDAV
	events, err := calendar_db.GetCalendarEvents(calendarConfig, startDate, endDate.Add(24*time.Hour))
	if err != nil {
		logger.Error("Failed to fetch calendar events", "error", err)
		// Continue with empty events list
		events = []calendar_db.Event{}
	}

	// Build calendar data structure
	type CalendarDay struct {
		Day          int
		Date         string // YYYY-MM-DD format for JavaScript
		IsOtherMonth bool
		IsToday      bool
		Events       []struct {
			UID                string
			Title              string
			AllDay             bool
			StartTimeFormatted string
		}
	}

	var calendarDays []CalendarDay
	today := time.Now()
	currentDate := startDate

	// Group events by date
	eventsByDate := make(map[string][]calendar_db.Event)
	for _, event := range events {
		dateKey := event.StartTime.Format("2006-01-02")
		eventsByDate[dateKey] = append(eventsByDate[dateKey], event)
	}

	// Build calendar grid
	for currentDate.Before(endDate.AddDate(0, 0, 1)) {
		day := CalendarDay{
			Day:          currentDate.Day(),
			Date:         currentDate.Format("2006-01-02"),
			IsOtherMonth: currentDate.Month() != month,
			IsToday:      currentDate.Format("2006-01-02") == today.Format("2006-01-02"),
		}

		// Add events for this day
		dateKey := currentDate.Format("2006-01-02")
		if dayEvents, ok := eventsByDate[dateKey]; ok {
			for _, event := range dayEvents {
				day.Events = append(day.Events, struct {
					UID                string
					Title              string
					AllDay             bool
					StartTimeFormatted string
				}{
					UID:                event.UID,
					Title:              event.Title,
					AllDay:             event.AllDay,
					StartTimeFormatted: event.StartTime.Format("3:04 PM"),
				})
			}
		}

		calendarDays = append(calendarDays, day)
		currentDate = currentDate.AddDate(0, 0, 1)
	}

	// Get upcoming events (next 30 days)
	upcomingStart := time.Now()
	upcomingEnd := upcomingStart.AddDate(0, 0, 30)
	upcomingEvents, err := calendar_db.GetCalendarEvents(calendarConfig, upcomingStart, upcomingEnd)
	if err != nil {
		logger.Error("Failed to fetch upcoming events", "error", err)
		upcomingEvents = []calendar_db.Event{}
	}

	// Prepare events JSON for modal
	eventsJSONData := []map[string]interface{}{}
	for _, e := range events {
		eventsJSONData = append(eventsJSONData, map[string]interface{}{
			"uid":         e.UID,
			"title":       e.Title,
			"description": e.Description,
			"location":    e.Location,
			"start_time":  e.StartTime,
			"end_time":    e.EndTime,
			"all_day":     e.AllDay,
		})
	}
	eventsJSON, _ := json.Marshal(eventsJSONData)

	// Template data
	data := struct {
		Username       string
		MonthYear      string
		PrevMonth      string
		NextMonth      string
		CalendarDays   []CalendarDay
		UpcomingEvents []calendar_db.Event
		EventsJSON     template.JS
		UserProfile    *UserProfile
	}{
		Username:       username,
		MonthYear:      firstDay.Format("January 2006"),
		PrevMonth:      firstDay.AddDate(0, -1, 0).Format("2006-01"),
		NextMonth:      firstDay.AddDate(0, 1, 0).Format("2006-01"),
		CalendarDays:   calendarDays,
		UpcomingEvents: upcomingEvents,
		EventsJSON:     template.JS(string(eventsJSON)),
	}

	// Get user profile for template
	profile, err := user_db.GetUserProfile(db, username)
	if err == nil {
		profile.IsAdmin = isAdmin(username)
		data.UserProfile = profile
	}

	t := template.New("base.html").Funcs(getTemplateFuncs())
	t, _ = t.ParseFiles("../templates/base.html", "../templates/calendar.html")
	err = t.Execute(w, data)
	if err != nil {
		logger.Error("Error executing calendar template", "error", err)
		return
	}
}

func servAddCalendarEvent(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form data
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	location := r.FormValue("location")
	startTimeStr := r.FormValue("start_time")
	endTimeStr := r.FormValue("end_time")
	allDay := r.FormValue("all_day") == "true"
	recurrenceRule := r.FormValue("recurrence_rule")

	if title == "" || startTimeStr == "" {
		http.Error(w, "Title and start time are required", http.StatusBadRequest)
		return
	}

	// Parse start time in local timezone
	var startTime time.Time
	if allDay {
		startTime, err = time.ParseInLocation("2006-01-02", startTimeStr, time.Local)
	} else {
		startTime, err = time.ParseInLocation("2006-01-02T15:04", startTimeStr, time.Local)
	}
	if err != nil {
		http.Error(w, "Invalid start time format", http.StatusBadRequest)
		return
	}

	// Parse end time (default to 1 hour after start if not provided)
	var endTime time.Time
	if endTimeStr == "" {
		if allDay {
			endTime = startTime.AddDate(0, 0, 1)
		} else {
			endTime = startTime.Add(time.Hour)
		}
	} else {
		if allDay {
			endTime, err = time.ParseInLocation("2006-01-02", endTimeStr, time.Local)
		} else {
			endTime, err = time.ParseInLocation("2006-01-02T15:04", endTimeStr, time.Local)
		}
		if err != nil {
			http.Error(w, "Invalid end time format", http.StatusBadRequest)
			return
		}
	}

	// Convert simple recurrence rule to RRULE format
	var rrule string
	if recurrenceRule != "" {
		rrule = convertToRRule(recurrenceRule)
	}

	// Create event
	event := calendar_db.Event{
		Title:       title,
		Description: description,
		Location:    location,
		StartTime:   startTime,
		EndTime:     endTime,
		AllDay:      allDay,
		CreatedBy:   username,
		RRule:       rrule,
	}

	err = calendar_db.CreateEvent(calendarConfig, event)
	if err != nil {
		logger.Error("Failed to create calendar event", "error", err)
		http.Error(w, "Failed to create event", http.StatusInternalServerError)
		return
	}

	// Redirect back to calendar
	http.Redirect(w, r, "/calendar", http.StatusSeeOther)
}

func servDeleteCalendarEvent(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uid := r.FormValue("uid")
	deleteSeries := r.FormValue("delete_series") == "true"

	if uid == "" {
		http.Error(w, "UID is required", http.StatusBadRequest)
		return
	}

	// Extract instance date from UID if it's a recurring event occurrence
	// Format: "originalUID-20260128" -> parse 20260128
	var instanceDate time.Time
	if idx := strings.LastIndex(uid, "-"); idx > 0 {
		datePart := uid[idx+1:]
		if len(datePart) == 8 {
			// Try to parse as date YYYYMMDD
			parsedDate, err := time.Parse("20060102", datePart)
			if err == nil {
				instanceDate = parsedDate
				logger.Info("Parsed instance date from UID", "uid", uid, "instanceDate", instanceDate.Format("2006-01-02"))
			}
		}
	}

	// Delete the event (or add EXDATE for single instance)
	err := calendar_db.DeleteEvent(calendarConfig, uid, deleteSeries, instanceDate)
	if err != nil {
		logger.Error("Failed to delete calendar event", "error", err, "username", username, "deleteSeries", deleteSeries)
		http.Error(w, "Failed to delete event", http.StatusInternalServerError)
		return
	}

	if deleteSeries {
		logger.Info("Deleted entire event series", "uid", uid)
	} else if !instanceDate.IsZero() {
		logger.Info("Added EXDATE for single recurring event instance", "uid", uid, "instanceDate", instanceDate.Format("2006-01-02"))
	} else {
		logger.Info("Deleted single event", "uid", uid)
	}

	// Return success
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Event deleted successfully",
	})
}

// convertToRRule converts simple recurrence format to proper RRULE
func convertToRRule(recurrenceRule string) string {
	// Format: "DAILY:10" -> "FREQ=DAILY;INTERVAL=1;COUNT=10"
	parts := strings.SplitN(recurrenceRule, ":", 2)
	if len(parts) != 2 {
		return ""
	}

	frequency := parts[0]
	count := parts[1]

	// Always include INTERVAL=1 for better compatibility
	return fmt.Sprintf("FREQ=%s;INTERVAL=1;COUNT=%s", frequency, count)
}

// createRecurringEvents creates recurring events based on a recurrence rule
// DEPRECATED: Now using RRULE directly in CalDAV
func createRecurringEvents(baseEvent calendar_db.Event, recurrenceRule string) error {
	// Parse recurrence rule (simple implementation)
	// Format: "DAILY:10" (10 days), "WEEKLY:4" (4 weeks), "MONTHLY:6" (6 months)
	parts := strings.SplitN(recurrenceRule, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid recurrence rule format")
	}

	frequency := parts[0]
	count := 0
	fmt.Sscanf(parts[1], "%d", &count)

	if count <= 0 || count > 100 {
		return fmt.Errorf("invalid recurrence count")
	}

	duration := baseEvent.EndTime.Sub(baseEvent.StartTime)

	for i := 1; i <= count; i++ {
		event := baseEvent
		event.UID = "" // Generate new UID

		switch frequency {
		case "DAILY":
			event.StartTime = baseEvent.StartTime.AddDate(0, 0, i)
			event.EndTime = event.StartTime.Add(duration)
		case "WEEKLY":
			event.StartTime = baseEvent.StartTime.AddDate(0, 0, i*7)
			event.EndTime = event.StartTime.Add(duration)
		case "MONTHLY":
			event.StartTime = baseEvent.StartTime.AddDate(0, i, 0)
			event.EndTime = event.StartTime.Add(duration)
		default:
			return fmt.Errorf("unsupported frequency: %s", frequency)
		}

		err := calendar_db.CreateEvent(calendarConfig, event)
		if err != nil {
			logger.Error("Failed to create recurring event instance", "error", err)
			// Continue with next instance
		}
	}

	return nil
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
		profile.NotifyOnNewArticles = r.FormValue("notify_on_new_articles") == "on"
		profile.NotifyOnNewMessages = r.FormValue("notify_on_new_messages") == "on"

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
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20) // 2MB limit for password changes
		if err := r.ParseForm(); err != nil {
			logger.Error("Form parsing error in changepassword", "error", err, "content-length", r.Header.Get("Content-Length"))
			http.Error(w, fmt.Sprintf("Form parsing error: %v", err), http.StatusBadRequest)
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

// getAllUsersFromHtpasswd reads all usernames from htpasswd file
func getAllUsersFromHtpasswd() ([]string, error) {
	htpasswdPath := "../blaze_auth/.htpasswd"
	data, err := os.ReadFile(htpasswdPath)
	if err != nil {
		return nil, err
	}

	var usernames []string
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if line != "" {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				usernames = append(usernames, parts[0])
			}
		}
	}

	return usernames, nil
}

// addUserToHtpasswd adds a new user to the htpasswd file
func addUserToHtpasswd(username, password string) error {
	htpasswdPath := "../blaze_auth/.htpasswd"

	// Check if user already exists
	data, err := os.ReadFile(htpasswdPath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, username+":") {
			return fmt.Errorf("user already exists")
		}
	}

	// Hash password using bcrypt
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	// Append user to htpasswd file
	newLine := username + ":" + string(hashedBytes) + "\n"
	file, err := os.OpenFile(htpasswdPath, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(newLine)
	return err
}

// updateUserPasswordInHtpasswd updates a user's password in htpasswd file
func updateUserPasswordInHtpasswd(username, newPassword string) error {
	htpasswdPath := "../blaze_auth/.htpasswd"

	// Hash new password using bcrypt
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	hashedPassword := string(hashedBytes)

	// Read htpasswd file
	data, err := os.ReadFile(htpasswdPath)
	if err != nil {
		return err
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
		return fmt.Errorf("user not found")
	}

	// Write back to file
	newContent := strings.Join(newLines, "\n") + "\n"
	err = os.WriteFile(htpasswdPath, []byte(newContent), 0600)
	return err
}

func servUserManagement(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	// Check if user is admin
	if !isAdmin(username) {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		logger.Warn("Non-admin user attempted to access user management", "username", username)
		return
	}

	type PageData struct {
		Error   string
		Success string
		Users   []user_db.UserProfile
	}

	// Get all usernames from htpasswd
	usernames, err := getAllUsersFromHtpasswd()
	if err != nil {
		logger.Error("Error reading htpasswd file", "error", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// Get user profiles for all users
	var users []user_db.UserProfile
	for _, uname := range usernames {
		profile, err := user_db.GetUserProfile(db, uname)
		if err != nil {
			logger.Error("Error getting user profile", "username", uname, "error", err)
			continue
		}
		users = append(users, *profile)
	}

	t := template.New("base.html").Funcs(getTemplateFuncs())
	t, _ = t.ParseFiles("../templates/base.html", "../templates/usermanagement.html")
	err = t.Execute(w, PageData{Users: users})
	if err != nil {
		logger.Error(err.Error())
		return
	}
}

func servNewUser(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	// Check if user is admin
	if !isAdmin(username) {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		logger.Warn("Non-admin user attempted to create new user", "username", username)
		return
	}

	type PageData struct {
		Error           string
		Success         bool
		CreatedUsername string
	}

	switch r.Method {
	case http.MethodGet:
		// Display new user form
		t := template.New("base.html").Funcs(getTemplateFuncs())
		t, _ = t.ParseFiles("../templates/base.html", "../templates/newuser.html")
		err := t.Execute(w, PageData{})
		if err != nil {
			logger.Error(err.Error())
			return
		}

	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20) // 2MB limit for user creation
		if err := r.ParseForm(); err != nil {
			logger.Error("Form parsing error in newuser", "error", err, "content-length", r.Header.Get("Content-Length"))
			http.Error(w, fmt.Sprintf("Form parsing error: %v", err), http.StatusBadRequest)
			return
		}

		newUsername := r.FormValue("username")
		password := r.FormValue("password")
		confirmPassword := r.FormValue("confirm_password")
		email := r.FormValue("email")

		// Validate passwords match
		if password != confirmPassword {
			t := template.New("base.html").Funcs(getTemplateFuncs())
			t, _ = t.ParseFiles("../templates/base.html", "../templates/newuser.html")
			t.Execute(w, PageData{Error: "Passwords do not match"})
			return
		}

		// Validate password length
		if len(password) < 6 {
			t := template.New("base.html").Funcs(getTemplateFuncs())
			t, _ = t.ParseFiles("../templates/base.html", "../templates/newuser.html")
			t.Execute(w, PageData{Error: "Password must be at least 6 characters"})
			return
		}

		// Validate username format
		if len(newUsername) < 3 {
			t := template.New("base.html").Funcs(getTemplateFuncs())
			t, _ = t.ParseFiles("../templates/base.html", "../templates/newuser.html")
			t.Execute(w, PageData{Error: "Username must be at least 3 characters"})
			return
		}

		// Add user to htpasswd file
		err := addUserToHtpasswd(newUsername, password)
		if err != nil {
			logger.Error("Error adding user to htpasswd", "username", newUsername, "error", err)
			t := template.New("base.html").Funcs(getTemplateFuncs())
			t, _ = t.ParseFiles("../templates/base.html", "../templates/newuser.html")
			t.Execute(w, PageData{Error: fmt.Sprintf("Error creating user: %s", err.Error())})
			return
		}

		// Create user profile
		profile := user_db.UserProfile{
			Username: newUsername,
			Handle:   newUsername,
			Email:    email,
		}
		err = user_db.UpdateUserProfile(db, &profile)
		if err != nil {
			logger.Error("Error creating user profile", "username", newUsername, "error", err)
			// Note: user is already in htpasswd, but profile creation failed
		}

		logger.Info("New user created", "username", newUsername, "by", username)

		// Show success message
		t := template.New("base.html").Funcs(getTemplateFuncs())
		t, _ = t.ParseFiles("../templates/base.html", "../templates/newuser.html")
		t.Execute(w, PageData{Success: true, CreatedUsername: newUsername})
	}
}

func servAdminResetPassword(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	// Check if user is admin
	if !isAdmin(username) {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		logger.Warn("Non-admin user attempted to reset password", "username", username)
		return
	}

	type PageData struct {
		Error          string
		Success        bool
		TargetUsername string
	}

	switch r.Method {
	case http.MethodGet:
		targetUsername := r.URL.Query().Get("username")
		if targetUsername == "" {
			http.Error(w, "Username required", http.StatusBadRequest)
			return
		}

		// Display password reset form
		t := template.New("base.html").Funcs(getTemplateFuncs())
		t, _ = t.ParseFiles("../templates/base.html", "../templates/adminresetpassword.html")
		err := t.Execute(w, PageData{TargetUsername: targetUsername})
		if err != nil {
			logger.Error(err.Error())
			return
		}

	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, 2<<20) // 2MB limit for password reset
		if err := r.ParseForm(); err != nil {
			logger.Error("Form parsing error in adminresetpassword", "error", err, "content-length", r.Header.Get("Content-Length"))
			http.Error(w, fmt.Sprintf("Form parsing error: %v", err), http.StatusBadRequest)
			return
		}

		targetUsername := r.FormValue("target_username")
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")

		// Validate passwords match
		if newPassword != confirmPassword {
			t := template.New("base.html").Funcs(getTemplateFuncs())
			t, _ = t.ParseFiles("../templates/base.html", "../templates/adminresetpassword.html")
			t.Execute(w, PageData{Error: "Passwords do not match", TargetUsername: targetUsername})
			return
		}

		// Validate password length
		if len(newPassword) < 6 {
			t := template.New("base.html").Funcs(getTemplateFuncs())
			t, _ = t.ParseFiles("../templates/base.html", "../templates/adminresetpassword.html")
			t.Execute(w, PageData{Error: "Password must be at least 6 characters", TargetUsername: targetUsername})
			return
		}

		// Update password in htpasswd
		err := updateUserPasswordInHtpasswd(targetUsername, newPassword)
		if err != nil {
			logger.Error("Error updating password", "username", targetUsername, "error", err)
			t := template.New("base.html").Funcs(getTemplateFuncs())
			t, _ = t.ParseFiles("../templates/base.html", "../templates/adminresetpassword.html")
			t.Execute(w, PageData{Error: fmt.Sprintf("Error updating password: %s", err.Error()), TargetUsername: targetUsername})
			return
		}

		logger.Info("Password reset by admin", "target_user", targetUsername, "admin", username)

		// Show success message
		t := template.New("base.html").Funcs(getTemplateFuncs())
		t, _ = t.ParseFiles("../templates/base.html", "../templates/adminresetpassword.html")
		t.Execute(w, PageData{Success: true, TargetUsername: targetUsername})
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

			// Check authorization: user must be the author or an admin
			if article.Author != username && !isAdmin(username) {
				logger.Warn("Unauthorized edit attempt", "user", username, "articleAuthor", article.Author, "articleID", articleID)
				http.Error(w, "You do not have permission to edit this article", http.StatusForbidden)
				return
			}

			logger.Debug("servArticle()[GET] - Edit existing article", "articleID", articleID, "title", article.Title)

			pageData := ArticleWithProfile{
				Article:       article,
				Profile:       profile,
				AvailableTags: blog_db.GetAllTags(db),
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
				Article:       Article{Title: ""},
				Profile:       profile,
				AvailableTags: blog_db.GetAllTags(db),
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

		// Set max memory for form parsing to 32MB (default is 32MB for multipart, but 10MB for regular forms)
		r.Body = http.MaxBytesReader(w, r.Body, 32<<20) // 32MB limit

		if err := r.ParseForm(); err != nil {
			logger.Error("Form parsing error", "error", err, "content-length", r.Header.Get("Content-Length"))
			http.Error(w, fmt.Sprintf("Form parsing error: %v", err), http.StatusBadRequest)
			return
		}

		// Validate required fields
		title := strings.TrimSpace(r.FormValue("title"))
		if title == "" {
			logger.Error("Title is required")
			http.Error(w, "Title is required", http.StatusBadRequest)
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

			// Check authorization: user must be the author or an admin
			if article.Author != username && !isAdmin(username) {
				logger.Warn("Unauthorized update attempt", "user", username, "articleAuthor", article.Author, "articleID", articleID)
				http.Error(w, "You do not have permission to edit this article", http.StatusForbidden)
				return
			}

			// Update fields
			article.Title = title
			article.Content = template.HTML(r.FormValue("content"))
			article.Tags = strings.TrimSpace(r.FormValue("tags"))
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
			article.Title = title
			article.Content = template.HTML(r.FormValue("content"))
			article.Tags = strings.TrimSpace(r.FormValue("tags"))
			article.Date = time.Now().Format("2006-01-02")
			article.Author = username
			article.IsNow = r.FormValue("is_now") == "on"
			article.IsPrivate = r.FormValue("is_private") == "on"
			article.IsIndex = r.FormValue("is_index") == "on"

			if ok := blog_db.SaveArticleWithNotifications(db, article, adminUsers); !ok {
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
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
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

	// Load the article to check ownership
	article, err := blog_db.GetArticleByID(db, articleID)
	if err != nil {
		logger.Error("Article not found:", "articleID", articleID)
		http.Error(w, "Article not found", http.StatusNotFound)
		return
	}

	// Check authorization: user must be the author or an admin
	if article.Author != username && !isAdmin(username) {
		logger.Warn("Unauthorized delete attempt", "user", username, "articleAuthor", article.Author, "articleID", articleID)
		http.Error(w, "You do not have permission to delete this article", http.StatusForbidden)
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
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
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

	logger.Debug("servArticleView()", "articleID", articleID, "title", article.Title, "article.ID", article.ID)

	// Get user profile to pass to template
	profile, _ := user_db.GetUserProfile(db, username)
	if profile != nil {
		profile.IsAdmin = isAdmin(username)
	}

	// Get reactions for this article
	reactions := blog_db.GetReactions(db, articleID)
	userReactions := blog_db.GetUserReactions(db, articleID, username)

	// Get comments for this article
	dbComments := blog_db.GetComments(db, articleID)
	comments := make([]Comment, len(dbComments))
	for i, c := range dbComments {
		comments[i] = Comment{
			ID:        c.ID,
			ArticleID: c.ArticleID,
			Username:  c.Username,
			Content:   c.Content,
			CreatedAt: c.CreatedAt.Format("2006-01-02 15:04"),
		}
	}

	pageData := ArticleWithProfile{
		Article:       article,
		Profile:       profile,
		Reactions:     reactions,
		UserReactions: userReactions,
		Comments:      comments,
	}

	t := template.New("base.html").Funcs(getTemplateFuncs())
	t, _ = t.ParseFiles("../templates/base.html", "../templates/article_view.html")
	err = t.Execute(w, pageData)

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

	// Check if there's a search query or tag filter
	searchQuery := r.URL.Query().Get("q")
	tagQuery := r.URL.Query().Get("tag")

	var articles []Article
	if len(tagQuery) > 0 {
		// Perform tag search
		pageData.Title = "Articles tagged with \"" + tagQuery + "\""
		pageData.TagQuery = tagQuery
		articles = blog_db.SearchArticlesByTag(db, tagQuery)
		logger.Debug("servArticles() - Tag search", "tag", tagQuery, "results", len(articles))
	} else if len(searchQuery) > 0 {
		// Perform general search
		pageData.Title = "Search Results for \"" + searchQuery + "\""
		pageData.SearchQuery = searchQuery
		articles = blog_db.SearchArticles(db, searchQuery)
		logger.Debug("servArticles() - Search", "query", searchQuery, "results", len(articles))
	} else {
		// Show all articles
		pageData.Title = "Decker News"
		articles = blog_db.GetAllArticles(db)
		logger.Debug("servArticles()")
	}

	blog_db.SortByDate(articles)
	pageData.Articles = enrichArticlesWithProfiles(articles)

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
	articles := blog_db.GetPrivateArticles(db, username)

	blog_db.SortByDate(articles)
	pageData.Articles = enrichArticlesWithProfiles(articles)

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
	articles := blog_db.GetMyArticles(db, username)

	blog_db.SortByDate(articles)
	pageData.Articles = enrichArticlesWithProfiles(articles)

	t := template.New("base.html").Funcs(getTemplateFuncs())
	t, _ = t.ParseFiles("../templates/base.html", "../templates/articles.html")
	err := t.Execute(w, pageData)

	if err != nil {
		logger.Error(err.Error())
		return
	}
}

func servReaction(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed baseAuth attempt")
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		logger.Error("Form parsing error")
		http.Error(w, "Form parsing error", http.StatusBadRequest)
		return
	}

	articleIDStr := r.FormValue("article_id")
	emoji := strings.TrimSpace(r.FormValue("emoji"))
	action := r.FormValue("action") // "add" or "remove"

	if articleIDStr == "" || emoji == "" || action == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	var articleID uint
	if _, err := fmt.Sscanf(articleIDStr, "%d", &articleID); err != nil {
		logger.Error("Invalid article ID:", "articleIDStr", articleIDStr, "error", err)
		http.Error(w, "Invalid article ID", http.StatusBadRequest)
		return
	}

	var success bool
	if action == "add" {
		success = blog_db.AddReaction(db, articleID, username, emoji)
	} else if action == "remove" {
		success = blog_db.RemoveReaction(db, articleID, username, emoji)
	} else {
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	if !success {
		http.Error(w, "Failed to process reaction", http.StatusInternalServerError)
		return
	}

	// Return success - JavaScript will handle UI update
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func servComment(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed baseAuth attempt")
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			logger.Error("Form parsing error")
			http.Error(w, "Form parsing error", http.StatusBadRequest)
			return
		}

		articleIDStr := r.FormValue("article_id")
		content := strings.TrimSpace(r.FormValue("content"))

		if articleIDStr == "" || content == "" {
			http.Error(w, "Missing required fields", http.StatusBadRequest)
			return
		}

		var articleID uint
		if _, err := fmt.Sscanf(articleIDStr, "%d", &articleID); err != nil {
			logger.Error("Invalid article ID:", "articleIDStr", articleIDStr, "error", err)
			http.Error(w, "Invalid article ID", http.StatusBadRequest)
			return
		}

		if !blog_db.AddCommentWithNotifications(db, articleID, username, content, adminUsers) {
			http.Error(w, "Failed to add comment", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	} else if r.Method == http.MethodDelete {
		// Extract comment ID from URL path
		path := strings.TrimPrefix(r.URL.Path, "/comment/")
		var commentID uint
		if _, err := fmt.Sscanf(path, "%d", &commentID); err != nil {
			logger.Error("Invalid comment ID:", "path", path, "error", err)
			http.Error(w, "Invalid comment ID", http.StatusBadRequest)
			return
		}

		if !blog_db.DeleteComment(db, commentID, username) {
			http.Error(w, "Failed to delete comment", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func servOnlineUsers(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get all users with their status
	allUsers, err := user_db.GetAllUsersWithStatus(db)
	if err != nil {
		logger.Error("Failed to get users with status", "error", err)
		http.Error(w, "Failed to get users with status", http.StatusInternalServerError)
		return
	}

	// Build response with user info including online/offline status
	type UserStatus struct {
		Username      string `json:"username"`
		Handle        string `json:"handle"`
		LastSeen      string `json:"last_seen"`
		IsOnline      bool   `json:"is_online"`
		IsCurrentUser bool   `json:"is_current_user"`
		MinutesAgo    int    `json:"minutes_ago"`
	}

	var response []UserStatus
	now := time.Now()
	onlineThreshold := 5 * time.Minute

	for _, user := range allUsers {
		lastSeenStr := ""
		isOnline := false
		minutesAgo := 0

		if user.LastSeen != nil {
			lastSeenStr = user.LastSeen.Format("2006-01-02 15:04:05")
			timeSince := now.Sub(*user.LastSeen)
			minutesAgo = int(timeSince.Minutes())
			isOnline = timeSince < onlineThreshold
		}

		response = append(response, UserStatus{
			Username:      user.Username,
			Handle:        user.Handle,
			LastSeen:      lastSeenStr,
			IsOnline:      isOnline,
			IsCurrentUser: user.Username == username,
			MinutesAgo:    minutesAgo,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func servUploadArticleImage(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt for image upload")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form with 10MB limit
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		logger.Error("Failed to parse multipart form", "error", err)
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		logger.Error("Failed to get image from form", "error", err)
		http.Error(w, "No image provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file type
	contentType := header.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, "File must be an image", http.StatusBadRequest)
		return
	}

	// Generate unique filename with timestamp
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".jpg"
	}
	filename := fmt.Sprintf("%d_%s%s", time.Now().Unix(), username, ext)

	// Create articles directory if it doesn't exist
	articlesDir := "../photos/articles"
	if err := os.MkdirAll(articlesDir, 0755); err != nil {
		logger.Error("Failed to create articles directory", "error", err)
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	// Save file
	filepath := filepath.Join(articlesDir, filename)
	dst, err := os.Create(filepath)
	if err != nil {
		logger.Error("Failed to create image file", "error", err)
		http.Error(w, "Failed to save image", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		logger.Error("Failed to write image file", "error", err)
		http.Error(w, "Failed to save image", http.StatusInternalServerError)
		return
	}

	logger.Info("Article image uploaded", "username", username, "filename", filename)

	// Return the URL
	imageURL := "/photos/articles/" + filename
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url": imageURL,
	})
}

func servChatSend(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req struct {
		ToUsername string `json:"to_username"`
		Content    string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ToUsername == "" || req.Content == "" {
		http.Error(w, "Missing required fields", http.StatusBadRequest)
		return
	}

	// Can't send message to yourself
	if req.ToUsername == username {
		http.Error(w, "Cannot send message to yourself", http.StatusBadRequest)
		return
	}

	// Send the message
	message, err := chat_db.SendMessage(db, username, req.ToUsername, req.Content)
	if err != nil {
		logger.Error("Failed to send message", "error", err)
		http.Error(w, "Failed to send message", http.StatusInternalServerError)
		return
	}

	// Send push notification to recipient
	go sendMessageNotification(db, username, req.ToUsername, req.Content)

	// Check if email notification should be sent
	go sendChatEmailNotification(db, username, req.ToUsername)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(message)
}

// sendMessageNotification sends a push notification for a new message
func sendMessageNotification(db *gorm.DB, fromUsername, toUsername, content string) {
	// Check if recipient wants notifications
	profile, err := user_db.GetUserProfile(db, toUsername)
	if err != nil || !profile.NotifyOnNewMessages {
		return
	}

	// Get sender's handle for display
	senderProfile, err := user_db.GetUserProfile(db, fromUsername)
	senderName := fromUsername
	if err == nil && senderProfile.Handle != "" {
		senderName = senderProfile.Handle
	}

	// Get recipient's push subscriptions
	subscriptions, err := push_db.GetUserSubscriptions(db, toUsername)
	if err != nil || len(subscriptions) == 0 {
		logger.Info("No push subscriptions for user", "username", toUsername)
		return
	}

	// Truncate message for notification
	notificationBody := content
	if len(notificationBody) > 100 {
		notificationBody = notificationBody[:97] + "..."
	}

	// Create notification payload
	notification := push_db.PushNotification{
		Title: "💬 " + senderName,
		Body:  notificationBody,
		Icon:  "/static/icons/icon-192x192.png",
		Data: map[string]interface{}{
			"url":  "/chat?with=" + fromUsername,
			"from": fromUsername,
			"type": "chat_message",
		},
	}

	payload, err := notification.ToJSON()
	if err != nil {
		logger.Error("Failed to create notification payload", "error", err)
		return
	}

	// In a full implementation, you would use a Web Push library here
	// For now, we'll just log what would be sent
	logger.Info("Push notification would be sent",
		"to", toUsername,
		"from", fromUsername,
		"subscriptions", len(subscriptions),
		"payload", payload,
	)

	// TODO: Implement actual Web Push sending using github.com/SherClockHolmes/webpush-go
	// Example:
	// for _, sub := range subscriptions {
	//     resp, err := webpush.SendNotification([]byte(payload), &webpush.Subscription{
	//         Endpoint: sub.Endpoint,
	//         Keys: webpush.Keys{
	//             P256dh: sub.P256dh,
	//             Auth:   sub.Auth,
	//         },
	//     }, &webpush.Options{
	//         VAPIDPublicKey:  vapidPublicKey,
	//         VAPIDPrivateKey: vapidPrivateKey,
	//         TTL:             30,
	//     })
	//
	//     if err != nil {
	//         logger.Error("Failed to send push notification", "error", err)
	//         // If subscription is no longer valid, delete it
	//         if resp != nil && (resp.StatusCode == 404 || resp.StatusCode == 410) {
	//             push_db.DeleteSubscription(db, sub.Endpoint)
	//         }
	//     }
	// }
}

// sendChatEmailNotification sends an email notification if user is offline/inactive
func sendChatEmailNotification(db *gorm.DB, fromUsername, toUsername string) {
	// Get recipient's profile
	recipientProfile, err := user_db.GetUserProfile(db, toUsername)
	if err != nil {
		logger.Error("Failed to get recipient profile for email notification", "username", toUsername, "error", err)
		return
	}

	// Check if recipient has email and wants notifications
	if recipientProfile.Email == "" || !recipientProfile.NotifyOnNewMessages {
		return
	}

	// Check if user has been inactive (no activity in last 5 minutes)
	// OR if messages are more than 1 day old and unread
	now := time.Now()
	inactiveThreshold := now.Add(-5 * time.Minute)

	isInactive := recipientProfile.LastSeen == nil || recipientProfile.LastSeen.Before(inactiveThreshold)

	if !isInactive {
		// User is active, don't send email
		return
	}

	// Get unread messages from sender that haven't been emailed yet
	unreadMessages, err := chat_db.GetUnreadMessagesForEmail(db, toUsername, fromUsername)
	if err != nil || len(unreadMessages) == 0 {
		return
	}

	// Check if oldest message is more than 1 day old
	oldestMessage := unreadMessages[0]
	oneDayAgo := now.Add(-24 * time.Hour)

	// Send email if user is inactive OR if messages are over a day old
	shouldSendEmail := isInactive && (oldestMessage.CreatedAt.Before(oneDayAgo) || len(unreadMessages) >= 3)

	if !shouldSendEmail {
		return
	}

	// Get sender's name
	senderProfile, err := user_db.GetUserProfile(db, fromUsername)
	senderName := fromUsername
	if err == nil && senderProfile.Handle != "" {
		senderName = senderProfile.Handle
	}

	// Prepare messages for email
	emailMessages := make([]blaze_email.ChatMessage, len(unreadMessages))
	messageIDs := make([]uint, len(unreadMessages))

	for i, msg := range unreadMessages {
		emailMessages[i] = blaze_email.ChatMessage{
			Content: msg.Content,
		}
		messageIDs[i] = msg.ID
	}

	// Build chat URL
	chatURL := fmt.Sprintf("https://blazemarker.com/chat?with=%s", fromUsername)

	// Send email
	recipientName := toUsername
	if recipientProfile.Handle != "" {
		recipientName = recipientProfile.Handle
	}

	err = blaze_email.SendChatNotification(
		recipientProfile.Email,
		recipientName,
		senderName,
		chatURL,
		emailMessages,
	)

	if err != nil {
		logger.Error("Failed to send chat email notification", "error", err, "to", toUsername, "from", fromUsername)
		return
	}

	// Mark messages as emailed
	err = chat_db.MarkEmailNotificationSent(db, messageIDs)
	if err != nil {
		logger.Error("Failed to mark messages as emailed", "error", err)
	}

	logger.Info("Chat email notification sent", "to", toUsername, "from", fromUsername, "messageCount", len(unreadMessages))
}

func servChatMessages(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get the other user from query parameter
	otherUser := r.URL.Query().Get("with")
	if otherUser == "" {
		http.Error(w, "Missing 'with' parameter", http.StatusBadRequest)
		return
	}

	// Get optional limit parameter (default 50)
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if _, err := fmt.Sscanf(limitStr, "%d", &limit); err != nil {
			limit = 50
		}
	}

	// Get messages
	messages, err := chat_db.GetRecentMessages(db, username, otherUser, limit)
	if err != nil {
		logger.Error("Failed to get messages", "error", err)
		http.Error(w, "Failed to get messages", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func servChatConversations(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get all conversations
	conversations, err := chat_db.GetConversations(db, username)
	if err != nil {
		logger.Error("Failed to get conversations", "error", err)
		http.Error(w, "Failed to get conversations", http.StatusInternalServerError)
		return
	}

	// Enrich with user handles from user_db
	for i := range conversations {
		profile, err := user_db.GetUserProfile(db, conversations[i].Username)
		if err == nil && profile != nil && profile.Handle != "" {
			conversations[i].Handle = profile.Handle
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(conversations)
}

func servChatMarkRead(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req struct {
		FromUsername string `json:"from_username"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.FromUsername == "" {
		http.Error(w, "Missing from_username", http.StatusBadRequest)
		return
	}

	// Mark messages as read
	if err := chat_db.MarkMessagesAsRead(db, username, req.FromUsername); err != nil {
		logger.Error("Failed to mark messages as read", "error", err)
		http.Error(w, "Failed to mark messages as read", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// Push notification handlers
func servPushSubscribe(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse subscription data
	var subscription push_db.SubscriptionData
	if err := json.NewDecoder(r.Body).Decode(&subscription); err != nil {
		logger.Error("Failed to parse subscription", "error", err)
		http.Error(w, "Invalid subscription data", http.StatusBadRequest)
		return
	}

	// Save subscription
	if err := push_db.SaveSubscription(db, username, subscription); err != nil {
		logger.Error("Failed to save subscription", "error", err)
		http.Error(w, "Failed to save subscription", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func servPushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var ok bool

	if ok, _ = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Error("Failed to parse request", "error", err)
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Delete subscription
	if err := push_db.DeleteSubscription(db, req.Endpoint); err != nil {
		logger.Error("Failed to delete subscription", "error", err)
		http.Error(w, "Failed to delete subscription", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func servPushVapidKey(w http.ResponseWriter, r *http.Request) {
	// Return VAPID public key for push subscriptions
	// For now, we'll generate this in the frontend using Web Crypto API
	// Or you can use a library like github.com/SherClockHolmes/webpush-go
	vapidPublicKey := os.Getenv("VAPID_PUBLIC_KEY")
	if vapidPublicKey == "" {
		vapidPublicKey = "BEl62iUYgUivxIkv69yViEuiBIa-Ib37gfKR_V-lU-xk31OKlFFNRD5Yt2Dw5N3Hy1QPj3Qn3T5j8kY7aDXl1W0" // Demo key
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"publicKey": vapidPublicKey})
}

// servShutdown gracefully shuts down the Blazemarker server
func servShutdown(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Only allow admin users to shutdown server
	if !isAdmin(username) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	logger.Info("Server shutdown initiated", "user", username)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "shutting down"})

	// Give response time to send
	go func() {
		time.Sleep(500 * time.Millisecond)
		os.Exit(0)
	}()
}

func servChatUnreadCount(w http.ResponseWriter, r *http.Request) {
	var username string
	var ok bool

	if ok, username = basicAuth(w, r); !ok {
		logger.Info("Failed basicAuth attempt")
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get unread count
	count, err := chat_db.GetUnreadCount(db, username)
	if err != nil {
		logger.Error("Failed to get unread count", "error", err)
		http.Error(w, "Failed to get unread count", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"count": count})
}

func main() {

	currentUser, err := user.Current()
	if err != nil {
		log.Fatalf(err.Error())
	}

	// Load admin users from config
	loadAdminUsers()

	// Load CalDAV configuration
	loadCalendarConfig()

	// Start session cleanup routine
	cleanupExpiredSessions()

	// TODO: Test general access to file system
	// TODO: Look for ways to lock down to specific directories
	http.Handle("/photos/galleries/", http.StripPrefix("/photos/galleries/", http.FileServer(http.Dir("../photos/galleries"))))
	http.Handle("/photos/avatars/", http.StripPrefix("/photos/avatars/", http.FileServer(http.Dir("../photos/avatars"))))
	http.Handle("/photos/articles/", http.StripPrefix("/photos/articles/", http.FileServer(http.Dir("../photos/articles"))))
	http.Handle("/bootstrap-5.3.0-dist/", http.StripPrefix("/bootstrap-5.3.0-dist/", http.FileServer(http.Dir("../bootstrap-5.3.0-dist"))))
	http.Handle("/tinymce/", http.StripPrefix("/tinymce/", http.FileServer(http.Dir("../tinymce"))))
	http.Handle("/css/", http.StripPrefix("/css/", http.FileServer(http.Dir("../css"))))
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("../static"))))

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

	http.HandleFunc("/apple-touch-icon-precomposed.png", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../static/apple-touch-icon.png")
	})

	http.HandleFunc("/favicon-16x16.png", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../static/favicon-16x16.png")
	})

	http.HandleFunc("/favicon-32x32.png", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../static/favicon-32x32.png")
	})

	http.HandleFunc("/offline.html", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../static/offline.html")
	})

	// TODO: Update /index to show photos, videos and blog and maybe an random photo, video or blog?  Or an about page
	http.HandleFunc("/index", servIndex)
	http.HandleFunc("/", servIndex)
	http.HandleFunc("/now", servNow)
	http.HandleFunc("/chat", servChat)
	http.HandleFunc("/calendar", servCalendar)
	http.HandleFunc("/calendar/event/add", servAddCalendarEvent)
	http.HandleFunc("/calendar/event/delete", servDeleteCalendarEvent)
	http.HandleFunc("/profile", servProfile)
	http.HandleFunc("/changepassword", servChangePassword)

	// Admin user management routes
	http.HandleFunc("/usermanagement", servUserManagement)
	http.HandleFunc("/newuser", servNewUser)
	http.HandleFunc("/adminresetpassword", servAdminResetPassword)

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
	http.HandleFunc("/reaction", servReaction)
	http.HandleFunc("/comment", servComment)
	http.HandleFunc("/comment/", servComment)
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

	// API endpoints
	http.HandleFunc("/api/upload-article-image", servUploadArticleImage)
	http.HandleFunc("/api/users/online", servOnlineUsers)
	http.HandleFunc("/api/chat/send", servChatSend)
	http.HandleFunc("/api/chat/messages", servChatMessages)
	http.HandleFunc("/api/chat/conversations", servChatConversations)
	http.HandleFunc("/api/chat/mark-read", servChatMarkRead)
	http.HandleFunc("/api/chat/unread-count", servChatUnreadCount)
	http.HandleFunc("/api/push/subscribe", servPushSubscribe)
	http.HandleFunc("/api/push/unsubscribe", servPushUnsubscribe)
	http.HandleFunc("/api/push/vapid-key", servPushVapidKey)

	// Server management
	http.HandleFunc("/api/shutdown", servShutdown)

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
