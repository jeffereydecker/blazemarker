package blaze_email

import (
	"bytes"
	"fmt"
	"html/template"
	"net"
	"net/smtp"
	"strings"

	"github.com/jeffereydecker/blazemarker/blaze_log"
)

var logger = blaze_log.GetLogger()

const (
	smtpHost = "localhost"
	smtpPort = "25"
	fromAddr = "noreply@blazemarker.com"
)

// ArticleNotification contains the data for article notification emails
type ArticleNotification struct {
	ArticleTitle   string
	ArticleContent string
	ArticleURL     string
	AuthorName     string
	RecipientName  string
}

// SendArticleNotification sends an email notification about a new article
func SendArticleNotification(toEmail, toName, articleTitle, articleContent, articleURL, authorName string) error {
	if toEmail == "" {
		return fmt.Errorf("recipient email is empty")
	}

	// Load and parse the email template
	tmpl, err := template.ParseFiles("../templates/email_article_notification.html")
	if err != nil {
		logger.Error("Failed to parse email template", "error", err)
		return err
	}

	// Prepare template data
	data := ArticleNotification{
		ArticleTitle:   articleTitle,
		ArticleContent: stripHTML(articleContent, 200), // First 200 chars, no HTML
		ArticleURL:     articleURL,
		AuthorName:     authorName,
		RecipientName:  toName,
	}

	// Execute template
	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		logger.Error("Failed to execute email template", "error", err)
		return err
	}

	// Prepare email headers and body
	subject := fmt.Sprintf("New Article: %s", articleTitle)
	msg := []byte(fmt.Sprintf(
		"From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"MIME-Version: 1.0\r\n"+
			"Content-Type: text/html; charset=UTF-8\r\n"+
			"\r\n"+
			"%s",
		fromAddr, toEmail, subject, body.String()))

	// Send email via localhost SMTP
	addr := fmt.Sprintf("%s:%s", smtpHost, smtpPort)

	// Connect to SMTP server
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		logger.Error("Failed to connect to SMTP server", "error", err)
		return err
	}
	defer conn.Close()

	// Create SMTP client (without TLS for localhost)
	client, err := smtp.NewClient(conn, smtpHost)
	if err != nil {
		logger.Error("Failed to create SMTP client", "error", err)
		return err
	}
	defer client.Close()

	// Set sender and recipient
	if err := client.Mail(fromAddr); err != nil {
		logger.Error("Failed to set sender", "error", err)
		return err
	}
	if err := client.Rcpt(toEmail); err != nil {
		logger.Error("Failed to set recipient", "error", err)
		return err
	}

	// Send message body
	wc, err := client.Data()
	if err != nil {
		logger.Error("Failed to initiate data transfer", "error", err)
		return err
	}
	defer wc.Close()

	if _, err := wc.Write(msg); err != nil {
		logger.Error("Failed to write message", "error", err)
		return err
	}

	logger.Info("Email notification sent", "to", toEmail, "article", articleTitle)
	return nil
}

// stripHTML removes HTML tags and truncates text to maxLen characters
func stripHTML(html string, maxLen int) string {
	// Simple HTML tag removal (for production, consider using a proper HTML parser)
	text := html

	// Remove common HTML tags
	replacements := []string{
		"<p>", "", "</p>", " ",
		"<div>", "", "</div>", " ",
		"<br>", " ", "<br/>", " ",
		"<h1>", "", "</h1>", " ",
		"<h2>", "", "</h2>", " ",
		"<h3>", "", "</h3>", " ",
		"<strong>", "", "</strong>", "",
		"<em>", "", "</em>", "",
		"<a", "", "</a>", "",
	}

	for i := 0; i < len(replacements); i += 2 {
		text = strings.ReplaceAll(text, replacements[i], replacements[i+1])
	}

	// Remove any remaining tags (basic approach)
	for strings.Contains(text, "<") && strings.Contains(text, ">") {
		start := strings.Index(text, "<")
		end := strings.Index(text, ">")
		if end > start {
			text = text[:start] + text[end+1:]
		} else {
			break
		}
	}

	// Trim whitespace
	text = strings.TrimSpace(text)
	text = strings.Join(strings.Fields(text), " ") // Normalize whitespace

	// Truncate to maxLen
	if len(text) > maxLen {
		text = text[:maxLen] + "..."
	}

	return text
}

// CommentNotification contains the data for comment notification emails
type CommentNotification struct {
	ArticleTitle       string
	ArticleURL         string
	CommenterName      string
	CommentContent     string
	RecipientName      string
	NotificationReason string
}

// SendCommentNotification sends an email notification about a new comment
func SendCommentNotification(toEmail, toName, articleTitle, articleURL, commenterName, commentContent, notificationReason string) error {
	if toEmail == "" {
		return fmt.Errorf("recipient email is empty")
	}

	// Load and parse the email template
	tmpl, err := template.ParseFiles("../templates/email_comment_notification.html")
	if err != nil {
		logger.Error("Failed to parse comment email template", "error", err)
		return err
	}

	// Prepare template data
	data := CommentNotification{
		ArticleTitle:       articleTitle,
		ArticleURL:         articleURL,
		CommenterName:      commenterName,
		CommentContent:     commentContent,
		RecipientName:      toName,
		NotificationReason: notificationReason,
	}

	// Execute template
	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		logger.Error("Failed to execute comment email template", "error", err)
		return err
	}

	// Prepare email headers and body
	subject := fmt.Sprintf("New comment on: %s", articleTitle)
	msg := []byte(fmt.Sprintf(
		"From: %s\r\n"+
			"To: %s\r\n"+
			"Subject: %s\r\n"+
			"MIME-Version: 1.0\r\n"+
			"Content-Type: text/html; charset=UTF-8\r\n"+
			"\r\n"+
			"%s",
		fromAddr, toEmail, subject, body.String()))

	// Send email via localhost SMTP
	addr := fmt.Sprintf("%s:%s", smtpHost, smtpPort)

	// Connect to SMTP server
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		logger.Error("Failed to connect to SMTP server", "error", err)
		return err
	}
	defer conn.Close()

	// Create SMTP client (without TLS for localhost)
	client, err := smtp.NewClient(conn, smtpHost)
	if err != nil {
		logger.Error("Failed to create SMTP client", "error", err)
		return err
	}
	defer client.Close()

	// Set sender and recipient
	if err := client.Mail(fromAddr); err != nil {
		logger.Error("Failed to set sender", "error", err)
		return err
	}
	if err := client.Rcpt(toEmail); err != nil {
		logger.Error("Failed to set recipient", "error", err)
		return err
	}

	// Send message body
	wc, err := client.Data()
	if err != nil {
		logger.Error("Failed to initiate data transfer", "error", err)
		return err
	}
	defer wc.Close()

	if _, err := wc.Write(msg); err != nil {
		logger.Error("Failed to write message", "error", err)
		return err
	}

	logger.Info("Comment notification sent", "to", toEmail, "article", articleTitle)
	return nil
}
