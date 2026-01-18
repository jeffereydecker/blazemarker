package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/jeffereydecker/blazemarker/mud_client"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Configuration
type Config struct {
	BlazemarkerURL   string // e.g. "http://localhost:3000" or "https://blazemarker.com"
	Username         string // Your Blazemarker username
	Password         string // Your Blazemarker password
	MudUsername      string // Your funklord username
	MudPassword      string // Your funklord password
	KeepAliveMinutes int    // How often to send keep-alive command (0 = disabled)
}

type ChatMessage struct {
	FromUsername string    `json:"from_username"`
	ToUsername   string    `json:"to_username"`
	Content      string    `json:"content"`
	CreatedAt    time.Time `json:"CreatedAt"`
}

type MudBridge struct {
	config      Config
	mudClient   *mud_client.MUDClient
	httpClient  *http.Client
	lastMsgTime time.Time
}

func main() {
	// Parse command-line flags
	blazemarkerURL := flag.String("url", "http://localhost:3000", "Blazemarker server URL")
	username := flag.String("user", "", "Your Blazemarker username")
	password := flag.String("pass", "", "Your Blazemarker password")
	mudUsername := flag.String("mud-user", "", "Your funklord username")
	mudPassword := flag.String("mud-pass", "", "Your funklord password")
	keepAlive := flag.Int("keep-alive", 5, "Send keep-alive command every N minutes (0=disabled)")
	flag.Parse()

	if *username == "" || *password == "" || *mudUsername == "" || *mudPassword == "" {
		fmt.Println("Usage: mud_bridge -user <username> -pass <password> -mud-user <mud_username> -mud-pass <mud_password>")
		fmt.Println("\nExample:")
		fmt.Println("  mud_bridge -user jdecker -pass mypass -mud-user jdecker -mud-pass mudpass")
		fmt.Println("\nOptional flags:")
		fmt.Println("  -url <url>          Blazemarker server URL (default: http://localhost:3000)")
		fmt.Println("  -keep-alive <mins>  Send keep-alive every N minutes (default: 5, 0=disabled)")
		return
	}

	config := Config{
		BlazemarkerURL:   *blazemarkerURL,
		Username:         *username,
		Password:         *password,
		MudUsername:      *mudUsername,
		MudPassword:      *mudPassword,
		KeepAliveMinutes: *keepAlive,
	}

	// Create a temporary in-memory database for the MUD client
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		fmt.Printf("Failed to create database: %v\n", err)
		return
	}

	bridge := &MudBridge{
		config:     config,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}

	fmt.Printf("Starting MUD Bridge...\n")
	fmt.Printf("Blazemarker: %s (user: %s)\n", config.BlazemarkerURL, config.Username)
	fmt.Printf("Funklord: funklord.com (user: %s)\n", config.MudUsername)
	if config.KeepAliveMinutes > 0 {
		fmt.Printf("Keep-alive: every %d minutes\n", config.KeepAliveMinutes)
	}

	// Create and start MUD client
	bridge.mudClient = mud_client.NewMUDClient(db, config.Username, config.MudUsername, config.MudPassword)
	err = bridge.mudClient.Start()
	if err != nil {
		fmt.Printf("Failed to start MUD client: %v\n", err)
		return
	}
	defer bridge.mudClient.Stop()

	fmt.Println("MUD client connected! Bridge is running...")

	// Start polling for commands from Blazemarker chat
	go bridge.pollForCommands()

	// Start keep-alive routine if enabled
	if config.KeepAliveMinutes > 0 {
		go bridge.keepAlive()
	}

	// Keep running
	select {}
}

// pollForCommands polls Blazemarker for messages from "funklord" conversation
func (b *MudBridge) pollForCommands() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		messages, err := b.fetchMessages()
		if err != nil {
			fmt.Printf("Error fetching messages: %v\n", err)
			continue
		}

		for _, msg := range messages {
			// Only process messages TO funklord from the user (commands to send to MUD)
			if msg.ToUsername == "funklord" && msg.FromUsername == b.config.Username {
				// Check if this is a new message we haven't processed
				if msg.CreatedAt.After(b.lastMsgTime) {
					fmt.Printf("[CMD] %s: %s\n", msg.FromUsername, msg.Content)
					err := b.mudClient.SendCommand(msg.Content)
					if err != nil {
						fmt.Printf("Error sending command to MUD: %v\n", err)
					}
					b.lastMsgTime = msg.CreatedAt
				}
			}
		}
	}
}

// keepAlive sends periodic commands to keep funklord session active
func (b *MudBridge) keepAlive() {
	ticker := time.NewTicker(time.Duration(b.config.KeepAliveMinutes) * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		fmt.Println("[KEEP-ALIVE] Sending look command...")
		err := b.mudClient.SendCommand("look")
		if err != nil {
			fmt.Printf("Keep-alive error: %v\n", err)
		}
	}
}

// fetchMessages retrieves messages from the funklord conversation
func (b *MudBridge) fetchMessages() ([]ChatMessage, error) {
	url := fmt.Sprintf("%s/api/chat/messages?with=funklord", b.config.BlazemarkerURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(b.config.Username, b.config.Password)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}

	var messages []ChatMessage
	err = json.NewDecoder(resp.Body).Decode(&messages)
	return messages, err
}

// sendMessage sends a message to Blazemarker chat (MUD output to user)
func (b *MudBridge) sendMessage(content string) error {
	url := fmt.Sprintf("%s/api/chat/send", b.config.BlazemarkerURL)

	payload := map[string]string{
		"to_username": b.config.Username,
		"content":     content,
	}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(b.config.Username, b.config.Password)

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}

	return nil
}
