package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// هيكل البيانات الأساسي
type Update struct {
	Message struct {
		MessageID int64  `json:"message_id"`
		Text      string `json:"text"`
		From      struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
		} `json:"from"`
		Chat struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"message"`
	CallbackQuery struct {
		ID   string `json:"id"`
		Data string `json:"data"`
		From struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
		} `json:"from"`
		Message struct {
			MessageID int64 `json:"message_id"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

func Handler(w http.ResponseWriter, r *http.Request) {
	token := os.Getenv("TELEGRAM_TOKEN")
	var update Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		return
	}

	// 1. أمر بدء اللعبة في المجموعة
	if strings.HasPrefix(update.Message.Text, "/xo") {
		sendNewGame(update.Message.Chat.ID, token)
	}

	// 2. معالجة الحركات (الضغط على الأزرار)
	if update.CallbackQuery.ID != "" {
		handleMove(update, token)
	}

	w.WriteHeader(http.StatusOK)
}

func sendNewGame(chatID int64, token string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	// لوحة البداية: 9 أزرار فارغة
	keyboard := renderBoard(".........", "X")
	payload := map[string]interface{}{
		"chat_id":      chatID,
		"text":         "🎮 تحدي XO جديد!\nالدور الآن على: **X**",
		"reply_markup": keyboard,
		"parse_mode":   "Markdown",
	}
	sendRequest(url, payload)
}

func handleMove(up Update, token string) {
	data := up.CallbackQuery.Data // الصيغة: "position|board|turn" -> "0|.........|X"
	parts := strings.Split(data, "|")
	pos := parts[0]
	board := []rune(parts[1])
	turn := parts[2]

	idx := int(pos[0] - '0')
	if board[idx] != '.' {
		answerCallback(up.CallbackQuery.ID, "❌ هذا المربع مشغول!", token)
		return
	}

	// تنفيذ الحركة
	board[idx] = rune(turn[0])
	newBoard := string(board)

	// التحقق من الفوز
	winner := checkWinner(newBoard)
	var statusText string
	var nextTurn string

	if winner != "" {
		if winner == "Draw" {
			statusText = "🤝 تعادل! لا يوجد فائز."
		} else {
			statusText = fmt.Sprintf("🎉 مبروك! الفائز هو: %s", winner)
		}
		nextTurn = "END"
	} else {
		if turn == "X" {
			nextTurn = "O"
		} else {
			nextTurn = "X"
		}
		statusText = fmt.Sprintf("🎮 الدور الآن على: %s", nextTurn)
	}

	// تحديث الرسالة الحالية
	editUrl := fmt.Sprintf("https://api.telegram.org/bot%s/editMessageText", token)
	editPayload := map[string]interface{}{
		"chat_id":    up.CallbackQuery.Message.Chat.ID,
		"message_id": up.CallbackQuery.Message.MessageID,
		"text":       statusText,
		"reply_markup": renderBoard(newBoard, nextTurn),
		"parse_mode": "Markdown",
	}
	sendRequest(editUrl, editPayload)
	answerCallback(up.CallbackQuery.ID, "", token)
}

func renderBoard(board string, nextTurn string) map[string]interface{} {
	rows := [][]map[string]string{}
	for i := 0; i < 3; i++ {
		row := []map[string]string{}
		for j := 0; j < 3; j++ {
			idx := i*3 + j
			char := string(board[idx])
			display := " "
			if char != "." {
				display = char
			}
			
			callbackData := "ignore"
			if nextTurn != "END" && char == "." {
				callbackData = fmt.Sprintf("%d|%s|%s", idx, board, nextTurn)
			}
			
			row = append(row, map[string]string{
				"text":          display,
				"callback_data": callbackData,
			})
		}
		rows = append(rows, row)
	}
	return map[string]interface{}{"inline_keyboard": rows}
}

func checkWinner(b string) string {
	lines := []string{
		b[0:3], b[3:6], b[6:9], // صفوف
		string([]byte{b[0], b[3], b[6]}), string([]byte{b[1], b[4], b[7]}), string([]byte{b[2], b[5], b[8]}), // أعمدة
		string([]byte{b[0], b[4], b[8]}), string([]byte{b[2], b[4], b[6]}), // أقطار
	}
	for _, l := range lines {
		if l == "XXX" { return "X" }
		if l == "OOO" { return "O" }
	}
	if !strings.Contains(b, ".") { return "Draw" }
	return ""
}

func sendRequest(url string, payload interface{}) {
	body, _ := json.Marshal(payload)
	http.Post(url, "application/json", bytes.NewBuffer(body))
}

func answerCallback(queryID string, text string, token string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/answerCallbackQuery", token)
	payload := map[string]string{"callback_query_id": queryID, "text": text}
	body, _ := json.Marshal(payload)
	http.Post(url, "application/json", bytes.NewBuffer(body))
}
