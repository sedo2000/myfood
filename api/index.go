package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"math/rand"
)

// هيكل البيانات الشامل
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

// تخزين النقاط والانتصارات (يفضل استخدام Redis في الإنتاج الضخم)
var stats = make(map[int64]int)

func Handler(w http.ResponseWriter, r *http.Request) {
	token := os.Getenv("TELEGRAM_TOKEN")
	var update Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		return
	}

	// الأوامر النصية
	text := update.Message.Text
	if strings.HasPrefix(text, "/xo") {
		if strings.Contains(text, "ai") {
			startAIGame(update.Message.Chat.ID, update.Message.From.ID, token)
		} else {
			initialGame(update.Message.Chat.ID, update.Message.From.ID, update.Message.From.FirstName, token)
		}
	} else if text == "/top" {
		showLeaderboard(update.Message.Chat.ID, token)
	}

	// معالجة الأزرار
	if update.CallbackQuery.ID != "" {
		processAction(update, token)
	}
	w.WriteHeader(http.StatusOK)
}

// --- نظام التحدي الجماعي ---

func initialGame(chatID int64, p1ID int64, name string, token string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	text := fmt.Sprintf("🎮 **تحدي XO جديد!**\n👤 المنظم: [%s](tg://user?id=%d)\n🆔 معرف اللاعب: `%d`\n\nبانتظار خصم شجاع للانضمام... ⚔️", name, p1ID, p1ID)
	
	keyboard := map[string]interface{}{
		"inline_keyboard": [][]map[string]interface{}{
			{{ "text": "انضمام للتحدي ⚡", "callback_data": fmt.Sprintf("join|%d", p1ID) }},
		},
	}
	sendReq(url, map[string]interface{}{"chat_id": chatID, "text": text, "reply_markup": keyboard, "parse_mode": "Markdown"})
}

func processAction(up Update, token string) {
	data := up.CallbackQuery.Data
	parts := strings.Split(data, "|")
	action := parts[0]

	if action == "join" {
		p1ID := parts[1]
		p2ID := fmt.Sprintf("%d", up.CallbackQuery.From.ID)
		if p1ID == p2ID {
			answer(up.CallbackQuery.ID, "❌ لا يمكنك تحدي نفسك يا صديقي!", token)
			return
		}
		renderGame(up, p1ID, p2ID, ".........", "X", token)
	} else if action == "move" {
		handleXOMove(up, parts, token)
	}
}

func renderGame(up Update, p1, p2, board, turn, token string) {
	editUrl := fmt.Sprintf("https://api.telegram.org/bot%s/editMessageText", token)
	
	currentID := p1
	if turn == "O" { currentID = p2 }
	
	text := fmt.Sprintf("🎮 **المباراة جارية!**\n\n📍 الدور على: [اللاعب %s](tg://user?id=%s)\n🆔 معرف الدور: `%s`", turn, currentID, currentID)
	
	payload := map[string]interface{}{
		"chat_id": up.CallbackQuery.Message.Chat.ID,
		"message_id": up.CallbackQuery.Message.MessageID,
		"text": text, "reply_markup": renderBoard(board, turn, p1, p2), "parse_mode": "Markdown",
	}
	sendReq(editUrl, payload)
}

func handleXOMove(up Update, parts []string, token string) {
	idx := int(parts[1][0] - '0')
	board := []rune(parts[2])
	turn := parts[3]
	p1, p2 := parts[4], parts[5]
	clicker := fmt.Sprintf("%d", up.CallbackQuery.From.ID)

	if (turn == "X" && clicker != p1) || (turn == "O" && clicker != p2) {
		answer(up.CallbackQuery.ID, "🚫 ليس دورك! انتظر من فضلك.", token)
		return
	}

	board[idx] = rune(turn[0])
	newB := string(board)
	winner := checkWinner(newB)

	if winner != "" {
		finishGame(up, winner, p1, p2, newB, token)
	} else {
		nextTurn := "O"
		if turn == "O" { nextTurn = "X" }
		renderGame(up, p1, p2, newB, nextTurn, token)
	}
}

func finishGame(up Update, winner, p1, p2, board, token string) {
	editUrl := fmt.Sprintf("https://api.telegram.org/bot%s/editMessageText", token)
	var status string
	if winner == "Draw" {
		status = "🤝 **نتيجة المباراة: تعادل!**\nتم منح كل لاعب نقطة واحدة."
		stats[up.CallbackQuery.From.ID]++ // تبسيط للنقاط
	} else {
		winID := p1
		if winner == "O" { winID = p2 }
		status = fmt.Sprintf("🎉 **الفائز المستحق: %s**\nالبطل: [اضغط هنا](tg://user?id=%s)\n🆔 المعرف: `%s`\nتمت إضافة 3 نقاط لرصيدك!", winner, winID, winID)
		// تحديث النقاط
		stats[up.CallbackQuery.From.ID] += 3
	}

	sendReq(editUrl, map[string]interface{}{
		"chat_id": up.CallbackQuery.Message.Chat.ID,
		"message_id": up.CallbackQuery.Message.MessageID,
		"text": status, "reply_markup": renderBoard(board, "END", p1, p2), "parse_mode": "Markdown",
	})
}

// --- نظام الذكاء الاصطناعي (AI Mode) ---
func startAIGame(chatID int64, pID int64, token string) {
	// منطق اللعب ضد البوت (يمكنك تطويره بخوارزمية Minimax)
}

func showLeaderboard(chatID int64, token string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	text := "🏆 **قائمة المتصدرين لعام 2026:**\n\n"
	for id, score := range stats {
		text += fmt.Sprintf("👤 اللاعب `%d`: %d نقطة\n", id, score)
	}
	sendReq(url, map[string]interface{}{"chat_id": chatID, "text": text, "parse_mode": "Markdown"})
}

// --- الدوال المساعدة ---

func renderBoard(board, next, p1, p2 string) map[string]interface{} {
	rows := [][]map[string]string{}
	for i := 0; i < 3; i++ {
		row := []map[string]string{}
		for j := 0; j < 3; j++ {
			n := i*3 + j
			char := string(board[n])
			txt := "▫️"
			if char == "X" { txt = "❌" } else if char == "O" { txt = "⭕" }
			
			data := "ignore"
			if next != "END" && char == "." {
				data = fmt.Sprintf("move|%d|%s|%s|%s|%s", n, board, next, p1, p2)
			}
			row = append(row, map[string]string{"text": txt, "callback_data": data})
		}
		rows = append(rows, row)
	}
	return map[string]interface{}{"inline_keyboard": rows}
}

func checkWinner(b string) string {
	patterns := []string{"012", "345", "678", "036", "147", "258", "048", "246"}
	for _, p := range patterns {
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
