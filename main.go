package main

import (
	openai "bdsc/clients"
	"fmt"
	"html/template"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var (
	tmpl        = template.Must(template.ParseFiles("templates/chat.html"))
	upgrader    = websocket.Upgrader{}
	clients     = make(map[*websocket.Conn]bool)
	broadcast   = make(chan string)
	clientMutex sync.Mutex

	openAIClient = openai.NewClient("gpt-3.5-turbo")
	chatHistory  []openai.ChatMessage
	historyMutex sync.Mutex
)

type PageData struct {
	Greeting template.HTML
}

func main() {
	http.HandleFunc("/", chatHandler)
	http.HandleFunc("/send", sendHandler)
	http.HandleFunc("/ws", wsHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	go handleMessages()

	fmt.Println("Listening on :8080")
	http.ListenAndServe(":8080", nil)
}

func chatHandler(w http.ResponseWriter, r *http.Request) {

	historyMutex.Lock()
	defer historyMutex.Unlock()

	chatHistory = append(chatHistory,
		openai.ChatMessage{Role: "system", Content: "You are a friendly and helpful assistant."},
	)
	resp, err := openAIClient.Chat(chatHistory)
	if err != nil {
		fmt.Println("GPT error:", err)
		return
	}

	chatHistory = append(chatHistory, openai.ChatMessage{Role: "assistant", Content: resp})
	html := fmt.Sprintf(`<div><b>Assistant:</b> %s</div>`, template.HTMLEscapeString(resp))
	tmpl.Execute(w, PageData{Greeting: template.HTML(html)})
}

func sendHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	message := r.FormValue("message")

	historyMutex.Lock()
	chatHistory = append(chatHistory, openai.ChatMessage{
		Role:    "user",
		Content: message,
	})
	historyMutex.Unlock()

	userHTML := fmt.Sprintf(`<div><b>You:</b> %s</div>`, template.HTMLEscapeString(message))
	broadcast <- userHTML

	historyMutex.Lock()
	resp, err := openAIClient.Chat(chatHistory)
	if err != nil {
		historyMutex.Unlock()
		http.Error(w, "Gpt failed", http.StatusInternalServerError)
		return
	}

	chatHistory = append(chatHistory, openai.ChatMessage{
		Role:    "assistant",
		Content: resp,
	})
	historyMutex.Unlock()

	gptHTML := fmt.Sprintf(`<div><b>Assistant:</b> %s</div>`, template.HTMLEscapeString(resp))
	broadcast <- gptHTML

	w.Header().Set("Content-Type", "text/html")
	fmt.Fprint(w, userHTML+gptHTML)
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	clientMutex.Lock()
	clients[conn] = true
	clientMutex.Unlock()

	defer func() {
		clientMutex.Lock()
		delete(clients, conn)
		clientMutex.Unlock()
		conn.Close()
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func handleMessages() {
	for {
		msg := <-broadcast
		fmt.Print("Message is brodcasting:", msg, "\n")
		clientMutex.Lock()
		for client := range clients {
			err := client.WriteMessage(websocket.TextMessage, []byte(msg))
			if err != nil {
				fmt.Print("error sending message to client")
				client.Close()
				delete(clients, client)
			}
		}
		clientMutex.Unlock()
	}
}
