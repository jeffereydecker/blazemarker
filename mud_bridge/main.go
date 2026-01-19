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
	MudUsername      string // Your Blazemarker funklord username
	MudPassword      string // Your Blazemarker funklord password
	MudWebUsername   string // Your funklord.com username
	MudWebPassword   string // Your funklord.com password
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
	mudUsername := flag.String("mud-user", "", "Your Blazemarker funklord username")
	mudPassword := flag.String("mud-pass", "", "Your Blazemarker funklord password")
	mudWebUsername := flag.String("mud-web-user", "", "Your funklord.com username")
	mudWebPassword := flag.String("mud-web-pass", "", "Your funklord.com password")
	keepAlive := flag.Int("keep-alive", 5, "Send keep-alive command every N minutes (0=disabled)")
	flag.Parse()

	if *username == "" || *password == "" || *mudUsername == "" || *mudPassword == "" || *mudWebUsername == "" || *mudWebPassword == "" {
		fmt.Println("Usage: mud_bridge -user <username> -pass <password> -mud-user <blazemarker_funklord_username> -mud-pass <blazemarker_funklord_password> -mud-web-user <funklord.com_username> -mud-web-pass <funklord.com_password>")
		fmt.Println("\nExample:")
		fmt.Println("  mud_bridge -user jdecker -pass mypass -mud-user funklord -mud-pass bm_funklord_pw -mud-web-user choff -mud-web-pass mud_pw")
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
		MudWebUsername:   *mudWebUsername,
		MudWebPassword:   *mudWebPassword,
		KeepAliveMinutes: *keepAlive,
	}

	// Create a temporary in-memory database for the MUD client
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		fmt.Printf("Failed to create database: %v\n", err)
		return
	}

	// Set lastMsgTime to now so only new messages are processed after startup
	bridge := &MudBridge{
		config:      config,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		lastMsgTime: time.Now(),
	}

	fmt.Printf("Starting MUD Bridge...\n")
	fmt.Printf("Blazemarker: %s (user: %s)\n", config.BlazemarkerURL, config.Username)
	fmt.Printf("Funklord: funklord.com (user: %s)\n", config.MudUsername)
	if config.KeepAliveMinutes > 0 {
		fmt.Printf("Keep-alive: every %d minutes\n", config.KeepAliveMinutes)
	}

	// Create and start MUD client
	bridge.mudClient = mud_client.NewMUDClient(db, config.Username, config.MudWebUsername, config.MudWebPassword)
	// Set callback to send MUD output to Blazemarker chat
	bridge.mudClient.OnMudOutput = func(msg string) {
		if msg != "" {
			fmt.Printf("[MUD->Blazemarker] (callback) %q\n", msg)
			err := bridge.sendMessage(msg)
			if err != nil {
				fmt.Printf("Failed to send MUD output to Blazemarker: %v\n", err)
			} else {
				fmt.Printf("Successfully sent to Blazemarker: %q\n", msg)
			}
		} else {
			fmt.Println("[MUD->Blazemarker] (callback) called with empty message")
		}
	}
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
					// Mark this message as read so it is not resent after restart
					markErr := b.markMessagesAsRead(msg.FromUsername)
					if markErr != nil {
						fmt.Printf("Error marking message as read: %v\n", markErr)
					}
					b.lastMsgTime = msg.CreatedAt
				}
			}
		}
	}
}

// markMessagesAsRead marks all messages from fromUsername as read for the current user
func (b *MudBridge) markMessagesAsRead(fromUsername string) error {
	url := fmt.Sprintf("%s/api/chat/mark-read", b.config.BlazemarkerURL)
	payload := map[string]string{"from_username": fromUsername}
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
		"to_username":   b.config.Username,
		"from_username": "funklord",
		"content":       content,
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
	// Authenticate as funklord for sending MUD output
	req.SetBasicAuth(b.config.MudUsername, b.config.MudPassword)

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
