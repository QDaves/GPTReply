package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	g "xabbo.b7c.io/goearth"
	"xabbo.b7c.io/goearth/shockwave/in"
	"xabbo.b7c.io/goearth/shockwave/out"
	"xabbo.b7c.io/goearth/shockwave/room"
)

//go:embed frontend/dist
var assets embed.FS
var isExtensionEnabled bool = false

type App struct {
	ctx             context.Context
	ext             *g.Ext
	users           map[int]*User
	roomMgr         *room.Manager
	userMap         map[string]Position
	chatLog         []ChatLogEntry
	settings        Settings
	lastMessageTime time.Time
	mu              sync.Mutex
}

type User struct {
	Index      int
	Name       string
	Figure     string
	Gender     string
	Custom     string
	X, Y       int
	Z          float64
	PoolFigure string
	BadgeCode  string
	Type       int
}

type Position struct{ X, Y int }

type ChatLogEntry struct {
	Time              time.Time
	Username, Message string
}

type Settings struct {
	Prefix, ChatGPTApiKey, ClaudeApiKey, ChatInstructions, OpenAIModel, ClaudeModel string
	ResponseDelay, PreviousChatCount, MaxTokens                                     int
	UseClaudeAPI, RespondToAll                                                      bool
	IgnoredUsers, BlacklistWords                                                    []string
	Temperature                                                                     float64
}

var (
	OpenAIModels = []string{"gpt-3.5-turbo", "gpt-4", "gpt-4o-mini", "gpt-4o", "gpt-4-turbo", "gpt-3.5-turbo-0125", "gpt-4-0613", "gpt-4-32k-0613", "gpt-3.5-turbo-0613", "gpt-3.5-turbo-16k", "gpt-4-vision-preview"}
	ClaudeModels = []string{"claude-3-opus-20240229", "claude-3-sonnet-20240229", "claude-3-haiku-20240307", "claude-3-5-sonnet-20240620"}
)

var ext = g.NewExt(g.ExtInfo{
	Title:       "GPT Reply",
	Description: "Reply with AI",
	Version:     "1.0",
	Author:      "QDave",
})

var app *App

func NewApp(ext *g.Ext, assets embed.FS) *App {
	return &App{
		ext:     ext,
		users:   make(map[int]*User),
		roomMgr: room.NewManager(ext),
		userMap: make(map[string]Position),
		chatLog: make([]ChatLogEntry, 0, 100),
		settings: Settings{
			Prefix: "+", ResponseDelay: 6, PreviousChatCount: 20, MaxTokens: 100, Temperature: 0.7,
			ChatInstructions: "You are in the Game Habbo. Keep responses short and under 200 characters.Use modern internet shortcut language.",
			OpenAIModel:      OpenAIModels[0], ClaudeModel: ClaudeModels[0],
		},
	}
}

func (a *App) debugm(msg string) {
	if os.Getenv("DEBUG") == "true" {
		log.Println(msg)
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.users = make(map[int]*User)
	a.loadsttngs()
	go a.updateR()
	a.setupExt()
}

func (a *App) loadsttngs() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	settingsPath := filepath.Join(filepath.Dir(exePath), "settings.json")

	data, err := ioutil.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err := a.SaveSettings(a.settings); err != nil {
			}
		}
		return
	}

	if err := json.Unmarshal(data, &a.settings); err != nil {
		return
	}
}

func (a *App) SaveSettings(newSettings Settings) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.settings = newSettings

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	settingsPath := filepath.Join(filepath.Dir(exePath), "settings.json")

	data, err := json.MarshalIndent(a.settings, "", "  ")
	if err != nil {
		return fmt.Errorf("%v", err)
	}

	err = ioutil.WriteFile(settingsPath, data, 0644)
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	runtime.EventsEmit(a.ctx, "settingsSaved")
	return nil
}

func (a *App) setupExt() {
	handlers := map[g.Identifier]func(*g.Intercept){
		in.OPC_OK: func(e *g.Intercept) { a.users = make(map[int]*User) },
		in.USERS:  a.husers,
		in.LOGOUT: a.removeuser,
		in.CHAT:   func(e *g.Intercept) { a.chatmessages(e, "CHAT") },
		in.CHAT_2: func(e *g.Intercept) { a.chatmessages(e, "WHISPER") },
		in.CHAT_3: func(e *g.Intercept) { a.chatmessages(e, "SHOUT") },
	}
	for header, handler := range handlers {
		a.ext.Intercept(header).With(handler)
	}
}

func (a *App) husers(e *g.Intercept) {
	count := e.Packet.ReadInt()
	for i := 0; i < count; i++ {
		var user User
		e.Packet.Read(&user)
		if user.Type == 1 {
			a.mu.Lock()
			a.users[user.Index] = &user
			a.mu.Unlock()
		}
	}
}

func (a *App) chatmessages(e *g.Intercept, _ string) {
	index := e.Packet.ReadInt()
	msg := e.Packet.ReadString()
	username := a.getUsername(index)
	if username == "" {
		username = "Unknown"
	}
	a.addchatlog(username, msg)
	if a.ignorereply(username, msg) {
		go a.rply2message(username, msg)
	}
}

func (a *App) ignorereply(username, msg string) bool {
	if !isExtensionEnabled {
		return false
	}
	if strings.Contains(msg, "[JOIN]") || strings.Contains(msg, "[LEFT]") {
		return false
	}
	return !a.isUserIgnored(username) && !a.containsBlacklistedWord(msg) &&
		(a.settings.RespondToAll || strings.HasPrefix(msg, a.settings.Prefix))
}

func (a *App) isUserIgnored(username string) bool {
	for _, ignored := range a.settings.IgnoredUsers {
		if strings.EqualFold(username, ignored) {
			return true
		}
	}
	return false
}

func (a *App) containsBlacklistedWord(msg string) bool {
	lowercaseMsg := strings.ToLower(msg)
	for _, word := range a.settings.BlacklistWords {
		if strings.Contains(lowercaseMsg, strings.ToLower(word)) {
			return true
		}
	}
	return false
}

func (a *App) addchatlog(username, message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.chatLog) >= 100 {
		a.chatLog = a.chatLog[1:]
	}
	a.chatLog = append(a.chatLog, ChatLogEntry{Time: time.Now(), Username: username, Message: message})
}

func (a *App) rply2message(username, message string) {
	a.mu.Lock()
	if time.Since(a.lastMessageTime) < time.Duration(a.settings.ResponseDelay)*time.Second {
		a.mu.Unlock()
		return
	}
	a.lastMessageTime = time.Now()
	a.mu.Unlock()

	var response string
	var err error
	if a.settings.UseClaudeAPI {
		response, err = a.getAPIResponse(username, message, true)
	} else {
		response, err = a.getAPIResponse(username, message, false)
	}
	if err != nil {
		return
	}
	if response == "" {
		a.logEvent("empty response API")
		return
	}
	for _, chunk := range a.splitIntoChunks(a.cleanResponse(response), 95) {
		a.ext.Send(out.SHOUT, chunk)
		time.Sleep(500 * time.Millisecond)
	}
}

func (a *App) getAPIResponse(username, message string, useClaudeAPI bool) (string, error) {
	client := &http.Client{}
	chatContext := a.getcntx()
	instructions := "You are a Habbo Origin GPT Bot made by QDave\n\n" + a.settings.ChatInstructions

	var reqBody interface{}
	var url, authHeader string

	if useClaudeAPI {
		reqBody = map[string]interface{}{
			"model":       a.settings.ClaudeModel,
			"system":      instructions,
			"messages":    []map[string]string{{"role": "user", "content": fmt.Sprintf("%s\n\nThe User %s asks: %s", chatContext, username, message)}},
			"max_tokens":  a.settings.MaxTokens,
			"temperature": a.settings.Temperature,
		}
		url = "https://api.anthropic.com/v1/messages"
		authHeader = "x-api-key"
	} else {
		reqBody = map[string]interface{}{
			"model":       a.settings.OpenAIModel,
			"messages":    []map[string]string{{"role": "system", "content": instructions}, {"role": "user", "content": fmt.Sprintf("%s\n\nThe User %s asks: %s", chatContext, username, message)}},
			"max_tokens":  a.settings.MaxTokens,
			"temperature": a.settings.Temperature,
		}
		url = "https://api.openai.com/v1/chat/completions"
		authHeader = "Authorization"
	}

	jsonData, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	req.Header.Set("Content-Type", "application/json")
	if useClaudeAPI {
		req.Header.Set(authHeader, a.settings.ClaudeApiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set(authHeader, "Bearer "+a.settings.ChatGPTApiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("%v", err)
	}

	if useClaudeAPI {
		content, ok := result["content"].([]interface{})
		if !ok || len(content) == 0 {
			return "", fmt.Errorf("empty Claude API response")
		}
		contentMap, ok := content[0].(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("fail Claude API response")
		}
		text, ok := contentMap["text"].(string)
		if !ok {
			return "", fmt.Errorf("not found Claude API response")
		}
		return text, nil
	} else {
		choices, ok := result["choices"].([]interface{})
		if !ok || len(choices) == 0 {
			return "", fmt.Errorf("error OpenAI API response")
		}
		choice, ok := choices[0].(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("error OpenAI API response")
		}
		message, ok := choice["message"].(map[string]interface{})
		if !ok {
			return "", fmt.Errorf("no message OpenAI API response")
		}
		content, ok := message["content"].(string)
		if !ok {
			return "", fmt.Errorf("no content OpenAI API response")
		}
		return content, nil
	}
}

func (a *App) getcntx() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var context strings.Builder
	context.WriteString("Previous Chatlog:\n")
	startIndex := len(a.chatLog) - a.settings.PreviousChatCount
	if startIndex < 0 {
		startIndex = 0
	}
	for _, entry := range a.chatLog[startIndex:] {
		context.WriteString(fmt.Sprintf("%s:%s:%s\n", entry.Time.Format("15:04:05"), entry.Username, entry.Message))
	}
	return context.String()
}

func (a *App) cleanResponse(input string) string {
	clean := make([]rune, 0, len(input))
	for _, r := range input {
		if r >= 32 && r <= 126 || r == 8364 || r == 163 || r == 165 {
			clean = append(clean, r)
		}
	}
	return string(clean)
}

func (a *App) splitIntoChunks(s string, chunkSize int) []string {
	var chunks []string
	runes := []rune(s)
	for i := 0; i < len(runes); i += chunkSize {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

func (a *App) removeuser(e *g.Intercept) {
	s := e.Packet.ReadString()
	index, err := strconv.Atoi(s)
	if err != nil {
		return
	}

	username := a.getUsername(index)
	if username != "" {
		leaveMsg := fmt.Sprintf("[LEFT] %s (ID: %d) left the room", username, index)
		a.debugm(leaveMsg)
		a.mu.Lock()
		delete(a.users, index)
		a.mu.Unlock()
	}
}

func (a *App) getUsername(index int) string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if user, ok := a.users[index]; ok {
		return user.Name
	}
	return ""
}

func (a *App) updateR() {
	for {
		newUsers := make(map[string]Position)
		a.roomMgr.Entities(func(ent room.Entity) bool {
			newUsers[ent.Name] = Position{X: ent.X, Y: ent.Y}
			return true
		})
		a.mu.Lock()
		a.userMap = newUsers
		a.mu.Unlock()
		time.Sleep(time.Second)
	}
}

func (a *App) logEvent(msg string) {
	a.debugm(msg)
	if !strings.Contains(msg, "[JOIN]") && !strings.Contains(msg, "[LEFT]") {
		runtime.EventsEmit(a.ctx, "logUpdate", msg)
	}
}

func (a *App) GetSettings() Settings {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.settings
}

func (a *App) UpdateSettings(newSettings Settings) {
	a.SaveSettings(newSettings)
}

func (a *App) GetChatLog() []ChatLogEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.chatLog
}

func main() {
	app = NewApp(ext, assets)
	setupExt()

	disconnectTime := time.Time{}
	reconnectAttempts := 0

	go func() {
		for {
			func() {
				defer func() {
					if r := recover(); r != nil {
						if disconnectTime.IsZero() {
							disconnectTime = time.Now()
						}
						reconnectAttempts++
						if time.Since(disconnectTime) > 2*time.Second {
							os.Exit(1)
						}
					}
				}()
				ext.Run()
				disconnectTime = time.Time{}
				reconnectAttempts = 0
			}()
			time.Sleep(1 * time.Second)
		}
	}()

	err := wails.Run(&options.App{
		Title:  "GPT Reply",
		Width:  800,
		Height: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		Bind: []interface{}{
			app,
		},
		StartHidden:       true,
		HideWindowOnClose: true,
		DisableResize:     true,
	})

	if err != nil {
		log.Fatal(err)
	}
}

func (a *App) ToggleExtension() bool {
	isExtensionEnabled = !isExtensionEnabled
	return isExtensionEnabled
}

func (a *App) GetExtensionState() bool {
	return isExtensionEnabled
}

func (a *App) ShowWindow() {
	runtime.WindowShow(a.ctx)
}
func setupExt() {
	ext.Initialized(func(e g.InitArgs) {
	})

	ext.Activated(func() {
		app.ShowWindow()
	})

	ext.Connected(func(e g.ConnectArgs) {
	})

	ext.Disconnected(func() {
	})
}
