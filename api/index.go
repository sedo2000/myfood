package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type Update struct {
	Message struct {
		MessageID int64 `json:"message_id"`
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

	if strings.HasPrefix(update.Message.Text, "/xo") {
		initialGame(update.Message.Chat.ID, update.Message.From.ID, update.Message.From.FirstName, token)
	}

	if update.CallbackQuery.ID != "" {
		processAction(update, token)
	}
	w.WriteHeader(http.StatusOK)
}

// إنشاء رسالة التحدي الأولى
func initialGame(chatID int64, player1ID int64, name string, token string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	text := fmt.Sprintf("🎮 **تحدي XO جديد!**\n👤 المنظم: [%s](tg://user?id=%d)\n\nبانتظار خصم للانضمام...", name, player1ID)
	
	keyboard := map[string]interface{}{
		"inline_keyboard": [][]map[string]interface{}{
			{{ "text": "انضمام للتحدي ⚔️", "callback_data": fmt.Sprintf("join|%d", player1ID) }},
		},
	}

	payload := map[string]interface{}{
		"chat_id": chatID, "text": text, "reply_markup": keyboard, "parse_mode": "Markdown",
	}
	sendReq(url, payload)
}

func processAction(up Update, token string) {
	data := up.CallbackQuery.Data
	parts := strings.Split(data, "|")
	action := parts[0]

	if action == "join" {
		p1ID := parts[1]
		p2ID := fmt.Sprintf("%d", up.CallbackQuery.From.ID)
		if p1ID == p2ID {
			answer(up.CallbackQuery.ID, "❌ لا يمكنك اللعب ضد نفسك!", token)
			return
		}
		startRealGame(up, p1ID, p2ID, token)
	} else if action == "move" {
		handleXOMove(up, parts, token)
	}
}

func startRealGame(up Update, p1ID, p2ID string, token string) {
	editUrl := fmt.Sprintf("https://api.telegram.org/bot%s/editMessageText", token)
	board := "........."
	text := "🎮 اللعبة بدأت!\n❌ الدور: اللاعب الأول"
	keyboard := renderBoard(board, "X", p1ID, p2ID)
	
	payload := map[string]interface{}{
		"chat_id": up.CallbackQuery.Message.Chat.ID,
		"message_id": up.CallbackQuery.Message.MessageID,
		"text": text, "reply_markup": keyboard, "parse_mode": "Markdown",
	}
	sendReq(editUrl, payload)
}

func handleXOMove(up Update, parts []string, token string) {
	// data format: move | index | board | turn | p1ID | p2ID
	idx := int(parts[1][0] - '0')
	board := []rune(parts[2])
	turn := parts[3]
	p1ID, p2ID := parts[4], parts[5]
	currentClicker := fmt.Sprintf("%d", up.CallbackQuery.From.ID)

	// التحقق من الدور
	if (turn == "X" && currentClicker != p1ID) || (turn == "O" && currentClicker != p2ID) {
		answer(up.CallbackQuery.ID, "🚫 انتظر دورك!", token)
		return
	}

	board[idx] = rune(turn[0])
	newB := string(board)
	winner := checkWinner(newB)

	var status string
	var next string
	if winner != "" {
		if winner == "Draw" { status = "🤝 تعادل!" } else { status = "🎉 الفائز: " + winner }
		next = "END"
	} else {
		if turn == "X" { next = "O" } else { next = "X" }
		status = "🎮 الدور الآن على: " + next
	}

	editUrl := fmt.Sprintf("https://api.telegram.org/bot%s/editMessageText", token)
	payload := map[string]interface{}{
		"chat_id": up.CallbackQuery.Message.Chat.ID,
		"message_id": up.CallbackQuery.Message.MessageID,
		"text": status, "reply_markup": renderBoard(newB, next, p1ID, p2ID), "parse_mode": "Markdown",
	}
	sendReq(editUrl, payload)
}

func renderBoard(board string, next, p1, p2 string) map[string]interface{} {
	rows := [][]map[string]string{}
	for i := 0; i < 3; i++ {
		row := []map[string]string{}
		for j := 0; j < 3; j++ {
			n := i*3 + j
			char := string(board[n])
			txt := " "
			if char != "." { txt = char }
			
			callData := "ignore"
			if next != "END" && char == "." {
				callData = fmt.Sprintf("move|%d|%s|%s|%s|%s", n, board, next, p1, p2)
			}
			row = append(row, map[string]string{"text": txt, "callback_data": callData})
		}
		rows = append(rows, row)
	}
	return map[string]interface{}{"inline_keyboard": rows}
}

// ... دوال checkWinner و sendReq و answer (نفس السابقة مع تعديل بسيط)
func checkWinner(b string) string {
	winPatterns := []string{"012", "345", "678", "036", "147", "258", "048", "246"}
	for _, p := range winPatterns {
		if b[p[0]-'0'] != '.' && b[p[0]-'0'] == b[p[1]-'0'] && b[p[1]-'0'] == b[p[2]-'0'] {
			return string(b[p[0]-'0'])
		}
	}
	if !strings.Contains(b, ".") { return "Draw" }
	return ""
}

func sendReq(url string, p interface{}) {
	b, _ := json.Marshal(p)
	http.Post(url, "application/json", bytes.NewBuffer(b))
}

func answer(id, text, token string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/answerCallbackQuery", token)
	p := map[string]string{"callback_query_id": id, "text": text}
	b, _ := json.Marshal(p)
	http.Post(url, "application/json", bytes.NewBuffer(b))
}
