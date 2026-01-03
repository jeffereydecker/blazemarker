package user_db

import (
	"github.com/jeffereydecker/blazemarker/blaze_log"
	"gorm.io/gorm"
)

var logger = blaze_log.GetLogger()

type UserProfile struct {
	gorm.Model
	Username   string `gorm:"uniqueIndex;not null"`
	Handle     string `gorm:"uniqueIndex"`
	Email      string
	Phone      string
	AvatarPath string
	IsAdmin    bool `gorm:"-"` // Not stored in DB, computed at runtime
}

func GetUserProfile(db *gorm.DB, username string) (*UserProfile, error) {
	db.AutoMigrate(&UserProfile{})

	var profile UserProfile
	result := db.Where("username = ?", username).First(&profile)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Create a default profile for new users
			profile = UserProfile{
				Username: username,
				Handle:   username,
			}
			db.Create(&profile)
			return &profile, nil
		}
		logger.Error("Error reading user profile:", "username", username, "error", result.Error)
		return nil, result.Error
	}

	return &profile, nil
}

func UpdateUserProfile(db *gorm.DB, profile *UserProfile) error {
	db.AutoMigrate(&UserProfile{})

	result := db.Save(profile)
	if result.Error != nil {
		logger.Error("Error updating user profile:", "username", profile.Username, "error", result.Error)
		return result.Error
	}

	return nil
}

func GetUserProfileByHandle(db *gorm.DB, handle string) (*UserProfile, error) {
	db.AutoMigrate(&UserProfile{})

	var profile UserProfile
	result := db.Where("handle = ?", handle).First(&profile)

	if result.Error != nil {
		logger.Error("Error reading user profile by handle:", "handle", handle, "error", result.Error)
		return nil, result.Error
	}

	return &profile, nil
}
