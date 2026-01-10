package push_db

import (
	"encoding/json"
	"time"

	"github.com/jeffereydecker/blazemarker/blaze_log"
	"gorm.io/gorm"
)

var logger = blaze_log.GetLogger()

// PushSubscription stores a user's push notification subscription
type PushSubscription struct {
	gorm.Model
	Username   string     `gorm:"index;not null" json:"username"`
	Endpoint   string     `gorm:"uniqueIndex;not null" json:"endpoint"`
	P256dh     string     `gorm:"not null" json:"p256dh"`
	Auth       string     `gorm:"not null" json:"auth"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	LastUsedAt time.Time  `gorm:"index" json:"last_used_at"`
}

// SubscriptionKeys represents the keys for a push subscription
type SubscriptionKeys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// SubscriptionData represents the full subscription data from the browser
type SubscriptionData struct {
	Endpoint string           `json:"endpoint"`
	Keys     SubscriptionKeys `json:"keys"`
}

// SaveSubscription saves or updates a push subscription for a user
func SaveSubscription(db *gorm.DB, username string, subscription SubscriptionData) error {
	db.AutoMigrate(&PushSubscription{})

	var existing PushSubscription
	result := db.Where("endpoint = ?", subscription.Endpoint).First(&existing)

	now := time.Now()

	if result.Error == gorm.ErrRecordNotFound {
		newSub := PushSubscription{
			Username:   username,
			Endpoint:   subscription.Endpoint,
			P256dh:     subscription.Keys.P256dh,
			Auth:       subscription.Keys.Auth,
			LastUsedAt: now,
		}

		if err := db.Create(&newSub).Error; err != nil {
			logger.Error("Failed to create push subscription", "username", username, "error", err)
			return err
		}

		logger.Info("Push subscription created", "username", username, "endpoint", subscription.Endpoint)
	} else if result.Error != nil {
		logger.Error("Failed to query push subscription", "error", result.Error)
		return result.Error
	} else {
		existing.Username = username
		existing.P256dh = subscription.Keys.P256dh
		existing.Auth = subscription.Keys.Auth
		existing.LastUsedAt = now

		if err := db.Save(&existing).Error; err != nil {
			logger.Error("Failed to update push subscription", "username", username, "error", err)
			return err
		}

		logger.Info("Push subscription updated", "username", username, "endpoint", subscription.Endpoint)
	}

	return nil
}

// GetUserSubscriptions retrieves all active push subscriptions for a user
func GetUserSubscriptions(db *gorm.DB, username string) ([]PushSubscription, error) {
	db.AutoMigrate(&PushSubscription{})

	var subscriptions []PushSubscription
	result := db.Where("username = ?", username).Find(&subscriptions)

	if result.Error != nil {
		logger.Error("Failed to get user subscriptions", "username", username, "error", result.Error)
		return nil, result.Error
	}

	return subscriptions, nil
}

// DeleteSubscription removes a push subscription
func DeleteSubscription(db *gorm.DB, endpoint string) error {
	db.AutoMigrate(&PushSubscription{})

	result := db.Where("endpoint = ?", endpoint).Delete(&PushSubscription{})

	if result.Error != nil {
		logger.Error("Failed to delete push subscription", "endpoint", endpoint, "error", result.Error)
		return result.Error
	}

	logger.Info("Push subscription deleted", "endpoint", endpoint)
	return nil
}

// CleanupExpiredSubscriptions removes subscriptions that have expired
func CleanupExpiredSubscriptions(db *gorm.DB) error {
	db.AutoMigrate(&PushSubscription{})

	now := time.Now()
	result := db.Where("expires_at IS NOT NULL AND expires_at < ?", now).Delete(&PushSubscription{})

	if result.Error != nil {
		logger.Error("Failed to cleanup expired subscriptions", "error", result.Error)
		return result.Error
	}

	if result.RowsAffected > 0 {
		logger.Info("Cleaned up expired subscriptions", "count", result.RowsAffected)
	}

	return nil
}

// PushNotification represents the data to send in a push notification
type PushNotification struct {
	Title string                 `json:"title"`
	Body  string                 `json:"body"`
	Icon  string                 `json:"icon,omitempty"`
	Badge string                 `json:"badge,omitempty"`
	Data  map[string]interface{} `json:"data,omitempty"`
}

// ToJSON converts a PushNotification to JSON string
func (p *PushNotification) ToJSON() (string, error) {
	bytes, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
