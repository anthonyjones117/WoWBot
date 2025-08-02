package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/sashabaranov/go-openai"
	uuid "github.com/satori/go.uuid"
)

type ClientManager struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

type Client struct {
	id     string
	socket *websocket.Conn
	send   chan []byte
}

type Message struct {
	Sender    string `json:"sender,omitempty"`
	Recipient string `json:"recipient,omitempty"`
	Content   string `json:"content,omitempty"`
}

type Memory map[string]interface{}

var manager = ClientManager{
	broadcast:  make(chan []byte),
	register:   make(chan *Client),
	unregister: make(chan *Client),
	clients:    make(map[*Client]bool),
}

var openaiClient *openai.Client

func loadMemory() (Memory, error) {
	file, err := os.Open("memory.json")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var memory Memory
	err = json.NewDecoder(file).Decode(&memory)
	if err != nil {
		return nil, err
	}
	return memory, nil
}

func init() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .env file not found")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}

	openaiClient = openai.NewClient(apiKey)
}

func askGPT(prompt, username string) (string, error) {
	memory, err := loadMemory()
	if err != nil {
		log.Println("Memory load error:", err)
	}

	var userSummary string
	for userKey, section := range memory {
		sectionMap, ok := section.(map[string]interface{})
		if !ok {
			continue
		}

		char, ok := sectionMap["character"].(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := char["name"].(string)
		race, _ := char["race"].(string)
		class, _ := char["class"].(string)

		if name != "" && race != "" && class != "" {
			userSummary += fmt.Sprintf("User %s plays a %s %s named %s.\n", userKey, race, class, name)
		}
	}

	systemPrompt := "You are a tactical assistant for World of Warcraft 2v2 PvP matches.\n"
	if userSummary != "" {
		systemPrompt += "User memory:\n" + userSummary
	}

	// PvP logic (you can keep this logic if relevant)
	if strings.Count(prompt, ",") >= 1 {

		systemPrompt += `The user team composition is already known and stored in memory. Make sure to consider our team's composition
		in your decisions.

		The user will provide two inputs and possibly the third:
		1. Opponent 1 spec and class
		2. Opponent 2 spec and class
		3. Arena map name

		Your job is to return a concise PvP strategy (can be read in under 15 seconds) that helps the user team win.

		Always respond in the following format, but don't include the Map section if no map was provided:

		---
		Strategy Summary

		[Opponent 1]
		- Role: [What role they play]
		- Kill Target?: [Yes/No and explain why]
		- Swap Logic: [When to consider switching to them]
		- Danger Abilities: [Important CDs or crowd control]
		- Team Synergy: [How they support their partner]

		[Opponent 2]
		- Role: ...
		- (Same format as above)

		Map : [Arena map name]
		- [One or two tips specific to this arena]`
	}

	resp, err := openaiClient.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT4Turbo,
			Messages: []openai.ChatCompletionMessage{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: prompt},
			},
		},
	)
	if err != nil {
		return "", err
	}
	return resp.Choices[0].Message.Content, nil
}

func (manager *ClientManager) start() {
	for {
		select {
		case conn := <-manager.register:
			manager.clients[conn] = true
			jsonMessage, _ := json.Marshal(&Message{Content: "/A new socket has connected."})
			manager.send(jsonMessage, conn)
		case conn := <-manager.unregister:
			if _, ok := manager.clients[conn]; ok {
				close(conn.send)
				delete(manager.clients, conn)
				jsonMessage, _ := json.Marshal(&Message{Content: "/A socket has disconnected."})
				manager.send(jsonMessage, conn)
			}
		case message := <-manager.broadcast:
			for conn := range manager.clients {
				select {
				case conn.send <- message:
				default:
					close(conn.send)
					delete(manager.clients, conn)
				}
			}
		}
	}
}

func (manager *ClientManager) send(message []byte, ignore *Client) {
	for conn := range manager.clients {
		if conn != ignore {
			conn.send <- message
		}
	}
}

func (c *Client) read() {
	defer func() {
		manager.unregister <- c
		c.socket.Close()
	}()

	for {
		_, message, err := c.socket.ReadMessage()
		if err != nil {
			manager.unregister <- c
			c.socket.Close()
			break
		}

		var incoming Message
		if err := json.Unmarshal(message, &incoming); err != nil {
			log.Println("Invalid message format:", err)
			continue
		}

		sender := incoming.Sender
		if sender == "" {
			sender = c.id
		}

		outgoing, _ := json.Marshal(&Message{Sender: sender, Content: incoming.Content})
		manager.broadcast <- outgoing

		if !strings.HasPrefix(strings.ToLower(incoming.Content), "/") {
			go func(input, username string) {
				reply, err := askGPT(input, username)
				if err != nil {
					log.Println("OpenAI error:", err)
					return
				}
				agentMsg, _ := json.Marshal(&Message{Sender: "Agent", Content: reply})
				manager.broadcast <- agentMsg
			}(incoming.Content, sender)
		}
	}
}

func (c *Client) write() {
	defer func() {
		c.socket.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.socket.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			c.socket.WriteMessage(websocket.TextMessage, message)
		}
	}
}

func main() {
	fmt.Println("Starting application...")
	go manager.start()
	http.HandleFunc("/ws", wsPage)
	http.HandleFunc("/exchange", withCORS(exchangeToken))        // <== wrapped
	http.HandleFunc("/profile", withCORS(fetchCharacterProfile)) // <== wrapped
	http.HandleFunc("/save-character", withCORS(saveCharacter))

	http.ListenAndServe("0.0.0.0:12345", nil)
}

func withCORS(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Allow your frontend origin
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:4200")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

		// Handle preflight request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		handler(w, r)
	}
}

func wsPage(res http.ResponseWriter, req *http.Request) {
	conn, err := (&websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}).Upgrade(res, req, nil)

	if err != nil {
		fmt.Println("WebSocket upgrade failed:", err)
		return
	}

	client := &Client{
		id:     uuid.NewV4().String(),
		socket: conn,
		send:   make(chan []byte),
	}

	manager.register <- client

	go client.read()
	go client.write()
}

func exchangeToken(w http.ResponseWriter, r *http.Request) {

	// Handle preflight OPTIONS request
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing code parameter", http.StatusBadRequest)
		return
	}

	clientID := os.Getenv("BLIZZARD_CLIENT_ID")
	clientSecret := os.Getenv("BLIZZARD_CLIENT_SECRET")
	redirectURI := "http://localhost:4200"

	fmt.Println("CLIENT ID:", clientID)
	fmt.Println("CLIENT SECRET:", clientSecret)

	data := fmt.Sprintf("grant_type=authorization_code&code=%s&redirect_uri=%s", code, redirectURI)

	req, err := http.NewRequest("POST", "https://oauth.battle.net/token", strings.NewReader(data))
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	req.SetBasicAuth(clientID, clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "Token exchange failed", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read token response", http.StatusInternalServerError)
		return
	}

	// Optional: decode the body if you want to log/inspect
	var tokenData map[string]interface{}
	json.Unmarshal(bodyBytes, &tokenData)
	fmt.Println("Token data:", tokenData)

	w.Header().Set("Content-Type", "application/json")
	w.Write(bodyBytes) // Return the original token JSON to frontend
}

func fetchCharacterProfile(w http.ResponseWriter, r *http.Request) {

	// Handle OPTIONS preflight
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Get the token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		http.Error(w, "Missing or invalid Authorization header", http.StatusUnauthorized)
		return
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")

	req, _ := http.NewRequest("GET", "https://us.api.blizzard.com/profile/user/wow", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Battlenet-Namespace", "profile-us")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "Failed to call Blizzard API", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.Write(body)
}

func fetchCharacterTalents(realm, character, token string) (map[string]interface{}, error) {
	url := fmt.Sprintf(
		"https://us.api.blizzard.com/profile/wow/character/%s/%s/specializations?namespace=profile-us&locale=en_US",
		realm, character,
	)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var talentData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&talentData); err != nil {
		return nil, err
	}
	return talentData, nil
}

func saveCharacter(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		// Parse JSON from request body
		var character struct {
			Name  string `json:"name"`
			Race  string `json:"race"`
			Class string `json:"class"`
			Realm string `json:"realm"`
			Token string `json:"token"`
		}
		err := json.NewDecoder(r.Body).Decode(&character)
		if err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		// Fetch talents from Blizzard API using realm, name, and token
		talents, err := fetchCharacterTalents(character.Realm, character.Name, character.Token)
		if err != nil {
			log.Println("Talent fetch failed:", err)
			talents = map[string]interface{}{"error": "Could not fetch talents"}
		}

		// Load current memory.json
		data, err := os.ReadFile("memory.json")
		if err != nil && !os.IsNotExist(err) {
			http.Error(w, "Failed to read memory file", http.StatusInternalServerError)
			return
		}

		var memory map[string]interface{}
		if len(data) > 0 {
			json.Unmarshal(data, &memory)
		} else {
			memory = make(map[string]interface{})
		}

		// Get the username from query param
		username := r.URL.Query().Get("user")
		if username == "" {
			http.Error(w, "Missing 'user' query parameter", http.StatusBadRequest)
			return
		}

		// Save character info with talents
		memory[username] = map[string]interface{}{
			"character": map[string]interface{}{
				"name":    character.Name,
				"race":    character.Race,
				"class":   character.Class,
				"realm":   character.Realm,
				"talents": talents,
			},
		}

		// Write back to file
		updated, _ := json.MarshalIndent(memory, "", "  ")
		os.WriteFile("memory.json", updated, 0644)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"message": "Character saved with talents"})
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
