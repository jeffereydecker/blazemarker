package chat_db

import (
	"time"

	"github.com/jeffereydecker/blazemarker/blaze_log"
	"gorm.io/gorm"
)

var logger = blaze_log.GetLogger()

// Message represents a chat message between two users
type Message struct {
	gorm.Model
	FromUsername            string     `gorm:"index;not null" json:"from_username"`
	ToUsername              string     `gorm:"index;not null" json:"to_username"`
	Content                 string     `gorm:"type:text;not null" json:"content"`
	IsRead                  bool       `gorm:"default:false;index" json:"is_read"`
	ReadAt                  *time.Time `json:"read_at,omitempty"`
	EmailNotificationSent   bool       `gorm:"default:false" json:"-"`
	EmailNotificationSentAt *time.Time `json:"-"`
}

// SendMessage creates a new chat message
func SendMessage(db *gorm.DB, fromUsername, toUsername, content string) (*Message, error) {
	db.AutoMigrate(&Message{})

	message := Message{
		FromUsername: fromUsername,
		ToUsername:   toUsername,
		Content:      content,
		IsRead:       false,
	}

	if result := db.Create(&message); result.Error != nil {
		logger.Error("Failed to send message", "from", fromUsername, "to", toUsername, "error", result.Error)
		return nil, result.Error
	}

	logger.Info("Message sent", "from", fromUsername, "to", toUsername, "messageID", message.ID)
	return &message, nil
}

// GetMessages retrieves all messages in a conversation between two users
func GetMessages(db *gorm.DB, username1, username2 string, limit int) ([]Message, error) {
	db.AutoMigrate(&Message{})

	var messages []Message

	// Get messages where user1 sent to user2 OR user2 sent to user1
	query := db.Where(
		"(from_username = ? AND to_username = ?) OR (from_username = ? AND to_username = ?)",
		username1, username2, username2, username1,
	).Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if result := query.Find(&messages); result.Error != nil {
		logger.Error("Failed to get messages", "user1", username1, "user2", username2, "error", result.Error)
		return nil, result.Error
	}

	return messages, nil
}

// GetRecentMessages retrieves recent messages (with optional limit to last N messages)
func GetRecentMessages(db *gorm.DB, username1, username2 string, limit int) ([]Message, error) {
	db.AutoMigrate(&Message{})

	var messages []Message

	// Get most recent messages in descending order, then reverse
	query := db.Where(
		"(from_username = ? AND to_username = ?) OR (from_username = ? AND to_username = ?)",
		username1, username2, username2, username1,
	).Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if result := query.Find(&messages); result.Error != nil {
		logger.Error("Failed to get recent messages", "user1", username1, "user2", username2, "error", result.Error)
		return nil, result.Error
	}

	// Reverse the slice to get chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// Conversation represents a conversation summary with another user
type Conversation struct {
	Username        string    `json:"username"`
	Handle          string    `json:"handle"`
	LastMessage     string    `json:"last_message"`
	LastMessageTime time.Time `json:"last_message_time"`
	UnreadCount     int       `json:"unread_count"`
	LastMessageFrom string    `json:"last_message_from"`
}

// GetConversations retrieves all conversations for a user with unread counts
func GetConversations(db *gorm.DB, username string) ([]Conversation, error) {
	db.AutoMigrate(&Message{})

	// Query to get unique conversation partners with their last message
	var results []struct {
		OtherUser       string
		LastMessage     string
		LastMessageTime time.Time
		LastMessageFrom string
		UnreadCount     int64
	}

	// This query gets the other user, last message content, time, and unread count
	query := `
		WITH conversations AS (
			SELECT 
				CASE 
					WHEN from_username = ? THEN to_username 
					ELSE from_username 
				END as other_user,
				content as last_message,
				created_at as last_message_time,
				from_username as last_message_from,
				ROW_NUMBER() OVER (
					PARTITION BY CASE 
						WHEN from_username = ? THEN to_username 
						ELSE from_username 
					END 
					ORDER BY created_at DESC
				) as rn
			FROM messages
			WHERE from_username = ? OR to_username = ?
		),
		unread_counts AS (
			SELECT 
				from_username as other_user,
				COUNT(*) as unread_count
			FROM messages
			WHERE to_username = ? AND is_read = false
			GROUP BY from_username
		)
		SELECT 
			c.other_user,
			c.last_message,
			c.last_message_time,
			c.last_message_from,
			COALESCE(u.unread_count, 0) as unread_count
		FROM conversations c
		LEFT JOIN unread_counts u ON c.other_user = u.other_user
		WHERE c.rn = 1
		ORDER BY c.last_message_time DESC
	`

	if err := db.Raw(query, username, username, username, username, username).Scan(&results).Error; err != nil {
		logger.Error("Failed to get conversations", "username", username, "error", err)
		return nil, err
	}

	// Convert to Conversation objects
	conversations := make([]Conversation, len(results))
	for i, r := range results {
		conversations[i] = Conversation{
			Username:        r.OtherUser,
			Handle:          r.OtherUser, // Will be enriched with actual handle from user_db if needed
			LastMessage:     r.LastMessage,
			LastMessageTime: r.LastMessageTime,
			LastMessageFrom: r.LastMessageFrom,
			UnreadCount:     int(r.UnreadCount),
		}
	}

	return conversations, nil
}

// MarkMessagesAsRead marks all messages from a specific user as read
func MarkMessagesAsRead(db *gorm.DB, toUsername, fromUsername string) error {
	db.AutoMigrate(&Message{})

	now := time.Now()
	result := db.Model(&Message{}).
		Where("to_username = ? AND from_username = ? AND is_read = ?", toUsername, fromUsername, false).
		Updates(map[string]interface{}{
			"is_read": true,
			"read_at": now,
		})

	if result.Error != nil {
		logger.Error("Failed to mark messages as read", "to", toUsername, "from", fromUsername, "error", result.Error)
		return result.Error
	}

	logger.Info("Messages marked as read", "to", toUsername, "from", fromUsername, "count", result.RowsAffected)
	return nil
}

// GetUnreadCount returns the total number of unread messages for a user
func GetUnreadCount(db *gorm.DB, username string) (int64, error) {
	db.AutoMigrate(&Message{})

	var count int64
	result := db.Model(&Message{}).
		Where("to_username = ? AND is_read = ?", username, false).
		Count(&count)

	if result.Error != nil {
		logger.Error("Failed to get unread count", "username", username, "error", result.Error)
		return 0, result.Error
	}

	return count, nil
}

// GetUnreadMessagesForEmail gets unread messages from a sender that haven't been emailed yet
func GetUnreadMessagesForEmail(db *gorm.DB, toUsername, fromUsername string) ([]Message, error) {
	db.AutoMigrate(&Message{})

	var messages []Message
	result := db.Where(
		"to_username = ? AND from_username = ? AND is_read = ? AND email_notification_sent = ?",
		toUsername, fromUsername, false, false,
	).Order("created_at ASC").Find(&messages)

	if result.Error != nil {
		logger.Error("Failed to get unread messages for email", "to", toUsername, "from", fromUsername, "error", result.Error)
		return nil, result.Error
	}

	return messages, nil
}

// MarkEmailNotificationSent marks messages as having had an email notification sent
func MarkEmailNotificationSent(db *gorm.DB, messageIDs []uint) error {
	if len(messageIDs) == 0 {
		return nil
	}

	db.AutoMigrate(&Message{})

	now := time.Now()
	result := db.Model(&Message{}).
		Where("id IN ?", messageIDs).
		Updates(map[string]interface{}{
			"email_notification_sent":    true,
			"email_notification_sent_at": now,
		})

	if result.Error != nil {
		logger.Error("Failed to mark email notifications as sent", "messageIDs", messageIDs, "error", result.Error)
		return result.Error
	}

	logger.Info("Email notifications marked as sent", "count", result.RowsAffected)
	return nil
}
