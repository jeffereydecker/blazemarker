package todo_db

import (
	"github.com/jeffereydecker/blazemarker/blaze_log"
	"gorm.io/gorm"
)

var logger = blaze_log.GetLogger()

// Todo represents a user todo item
type Todo struct {
	gorm.Model
	Username  string `json:"username" gorm:"index"`
	Text      string `json:"text" gorm:"type:text"`
	Completed bool   `json:"completed" gorm:"index"`
}

// GetTodos returns todos for a user ordered by newest first
func GetTodos(db *gorm.DB, username string) ([]Todo, error) {
	if err := db.AutoMigrate(&Todo{}); err != nil {
		logger.Error("AutoMigrate failed", "error", err)
		return nil, err
	}
	var todos []Todo
	res := db.Where("username = ?", username).Order("created_at DESC").Find(&todos)
	if res.Error != nil {
		logger.Error("GetTodos failed", "error", res.Error)
		return nil, res.Error
	}
	return todos, nil
}

// AddTodo creates a new todo for a user
func AddTodo(db *gorm.DB, username, text string) (*Todo, error) {
	if err := db.AutoMigrate(&Todo{}); err != nil {
		logger.Error("AutoMigrate failed", "error", err)
		return nil, err
	}
	t := &Todo{Username: username, Text: text}
	res := db.Create(t)
	if res.Error != nil {
		logger.Error("AddTodo failed", "error", res.Error)
		return nil, res.Error
	}
	return t, nil
}

// DeleteTodo removes a todo by id for the given user
func DeleteTodo(db *gorm.DB, username string, id uint) error {
	if err := db.AutoMigrate(&Todo{}); err != nil {
		logger.Error("AutoMigrate failed", "error", err)
		return err
	}
	res := db.Where("id = ? AND username = ?", id, username).Delete(&Todo{})
	if res.Error != nil {
		logger.Error("DeleteTodo failed", "error", res.Error)
		return res.Error
	}
	return nil
}

// UpdateTodo updates the text and completed state of a todo owned by username
func UpdateTodo(db *gorm.DB, username string, id uint, text string, completed bool) (*Todo, error) {
	if err := db.AutoMigrate(&Todo{}); err != nil {
		logger.Error("AutoMigrate failed", "error", err)
		return nil, err
	}
	var t Todo
	res := db.First(&t, "id = ? AND username = ?", id, username)
	if res.Error != nil {
		logger.Error("UpdateTodo find failed", "error", res.Error)
		return nil, res.Error
	}
	t.Text = text
	t.Completed = completed
	if err := db.Save(&t).Error; err != nil {
		logger.Error("UpdateTodo save failed", "error", err)
		return nil, err
	}
	return &t, nil
}
