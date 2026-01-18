package mud_client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jeffereydecker/blazemarker/blaze_log"
	"github.com/jeffereydecker/blazemarker/chat_db"

	"github.com/chromedp/chromedp"
	"gorm.io/gorm"
)

var logger = blaze_log.GetLogger()

type MUDClient struct {
	ctx            context.Context
	cancel         context.CancelFunc
	username       string
	running        bool
	mu             sync.Mutex
	lastChildCount int // Track number of child elements to detect individual messages
	db             *gorm.DB
	mudUsername    string
	mudPassword    string
	userForChat    string // The Blazemarker user this MUD is for
}

// NewMUDClient creates a new MUD client instance
func NewMUDClient(db *gorm.DB, blazemarkerUser, mudUsername, mudPassword string) *MUDClient {
	return &MUDClient{
		db:          db,
		username:    blazemarkerUser,
		mudUsername: mudUsername,
		mudPassword: mudPassword,
		userForChat: blazemarkerUser,
	}
}

// Start launches the headless browser and connects to funklord.com
func (m *MUDClient) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("MUD client already running")
	}
	m.running = true
	m.mu.Unlock()

	// Create chromedp context
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)
	allocCtx, _ := chromedp.NewExecAllocator(context.Background(), opts...)
	m.ctx, m.cancel = chromedp.NewContext(allocCtx)

	// Navigate to funklord.com and login using conversational interface
	err := chromedp.Run(m.ctx,
		chromedp.Navigate("https://funklord.com"),
		chromedp.WaitVisible(`textarea[name="UserInput"]`, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second), // Wait for page to fully load
		// Send username
		chromedp.SetValue(`textarea[name="UserInput"]`, m.mudUsername, chromedp.ByQuery),
		chromedp.SendKeys(`textarea[name="UserInput"]`, "\r", chromedp.ByQuery), // Press Enter
		chromedp.Sleep(2*time.Second),                                           // Wait for username prompt response
		// Send password
		chromedp.SetValue(`textarea[name="UserInput"]`, m.mudPassword, chromedp.ByQuery),
		chromedp.SendKeys(`textarea[name="UserInput"]`, "\r", chromedp.ByQuery), // Press Enter
		chromedp.Sleep(2*time.Second),                                           // Wait for login to complete
	)
	if err != nil {
		m.Stop()
		return fmt.Errorf("failed to login to funklord.com: %w", err)
	}

	logger.Info("MUD client connected to funklord.com for user: " + m.username)

	// Start monitoring loop
	go m.monitorOutput()

	return nil
}

// Stop terminates the headless browser session
func (m *MUDClient) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return
	}

	if m.cancel != nil {
		m.cancel()
	}
	m.running = false
	logger.Info("MUD client stopped for user: " + m.username)
}

// IsRunning returns whether the client is active
func (m *MUDClient) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// monitorOutput continuously checks for new MUD output
func (m *MUDClient) monitorOutput() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkForNewOutput()
		}
	}
}

// checkForNewOutput retrieves text from the MUD and sends to chat if new
func (m *MUDClient) checkForNewOutput() {
	var content string
	var childCount int

	// Try to get child element count to see if funklord uses individual message elements
	err := chromedp.Run(m.ctx,
		chromedp.Text(`#TextReceived`, &content, chromedp.ByID),
		chromedp.Evaluate(`document.querySelectorAll('#TextReceived > *').length`, &childCount),
	)
	if err != nil {
		logger.Error("Failed to read MUD output: " + err.Error())
		return
	}

	// Debug: Log which method we're using
	if childCount > 0 {
		logger.Info(fmt.Sprintf("MUD using child elements method: childCount=%d, lastChildCount=%d", childCount, m.lastChildCount))
	} else {
		logger.Info("MUD using text diff fallback (no child elements detected)")
	}

	// If funklord uses child elements for each message, track those instead of text diff
	if childCount > 0 && childCount != m.lastChildCount {
		// Get only new child elements
		var newMessages []string
		err = chromedp.Run(m.ctx,
			chromedp.Evaluate(fmt.Sprintf(`
				Array.from(document.querySelectorAll('#TextReceived > *'))
					.slice(%d)
					.map(el => el.textContent)
			`, m.lastChildCount), &newMessages),
		)
		if err == nil && len(newMessages) > 0 {
			for _, msg := range newMessages {
				if msg != "" {
					_, err = chat_db.SendMessage(m.db, "funklord", m.userForChat, msg)
					if err != nil {
						logger.Error("Failed to send MUD output to chat: " + err.Error())
					} else {
						logger.Info("New MUD message sent to chat: " + msg)
					}
				}
			}
			m.lastChildCount = childCount
			return
		}
	}
}

// SendCommand sends a command from the user to the MUD
func (m *MUDClient) SendCommand(command string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return fmt.Errorf("MUD client not running")
	}

	err := chromedp.Run(m.ctx,
		chromedp.SetValue(`textarea[name="UserInput"]`, command, chromedp.ByQuery),
		chromedp.SendKeys(`textarea[name="UserInput"]`, "\r", chromedp.ByQuery), // Press Enter
	)
	if err != nil {
		return fmt.Errorf("failed to send command to MUD: %w", err)
	}

	logger.Info(fmt.Sprintf("Command sent to MUD: %s", command))
	return nil
}
