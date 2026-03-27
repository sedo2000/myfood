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
		MessageID int64  `json:"message_id"`
		Text      string `json:"text"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From struct {
			ID int64 `json:"id"`
		} `json:"from"`
	} `json:"message"`
}

func Handler(w http.ResponseWriter, r *http.Request) {
	token := os.Getenv("TELEGRAM_TOKEN")
	var update Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		return
	}

	text := update.Message.Text
	chatID := update.Message.Chat.ID
	userID := update.Message.From.ID

	// 1. الترحيب
	if text == "/start" {
		sendMsg(chatID, "مرحباً بك! 📥\nأرسل رابط فيديو يوتيوب وسأقوم بمحاولة معالجته لك.\n\n⚠️ ملاحظة: الفيديوهات الطويلة قد لا تعمل بسبب قيود السيرفر.", token)
	}

	// 2. معالجة روابط يوتيوب
	if strings.Contains(text, "youtube.com") || strings.Contains(text, "youtu.be") {
		sendMsg(chatID, fmt.Sprintf("⏳ جاري تحليل الرابط للمستخدم: `%d`...\nيرجى الانتظار.", userID), token)
		
		// هنا يتم استدعاء دالة التحميل (منطقياً)
		// go downloadAndSend(chatID, text, token)
		
		sendMsg(chatID, "❌ عذراً! التحميل المباشر يتطلب سيرفر VPS.\nبسبب قيود Vercel، يمكنني فقط تزويدك بروابط تحميل خارجية حالياً.", token)
	}

	w.WriteHeader(http.StatusOK)
}

func sendMsg(chatID int64, text string, token string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}
	body, _ := json.Marshal(payload)
	http.Post(url, "application/json", bytes.NewBuffer(body))
}
