package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// هيكل البيانات لاستقبال التحديثات من تلجرام
type Update struct {
	Message struct {
		MessageID int64 `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		Photo []interface{} `json:"photo"`
	} `json:"message"`
	CallbackQuery struct {
		ID      string `json:"id"`
		Data    string `json:"data"` // سيحتوي على التصنيف: photoID_tag
		Message struct {
			MessageID int64 `json:"message_id"`
			Chat      struct {
				ID int64 `json:"id"`
			} `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

// متغير لتخزين التصويت مؤقتاً (في بيئة Serverless يفضل استخدام Database لاحقاً)
// هنا نستخدم الخريطة للتوضيح البرمجي
var votes = make(map[string]map[string]int) 

func Handler(w http.ResponseWriter, r *http.Request) {
	token := os.Getenv("TELEGRAM_TOKEN")
	var update Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		return
	}

	// 1. إذا كانت الرسالة "صورة"
	if len(update.Message.Photo) > 0 {
		sendClassificationButtons(update.Message.Chat.ID, update.Message.MessageID, token)
	}

	// 2. إذا ضغط العضو على زر (تصنيف)
	if update.CallbackQuery.ID != "" {
		handleVote(update, token)
	}

	w.WriteHeader(http.StatusOK)
}

// دالة إرسال الأزرار تحت الصورة
func sendClassificationButtons(chatID int64, msgID int64, token string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	
	keyboard := map[string]interface{}{
		"inline_keyboard": [][]map[string]string{
			{
				{"text": "طبيعة 🌿", "callback_data": fmt.Sprintf("%d_nature", msgID)},
				{"text": "طعام 🍔", "callback_data": fmt.Sprintf("%d_food", msgID)},
			},
			{
				{"text": "يوميات 🤳", "callback_data": fmt.Sprintf("%d_daily", msgID)},
				{"text": "مضحك 😂", "callback_data": fmt.Sprintf("%d_funny", msgID)},
			},
		},
	}
	kbJson, _ := json.Marshal(keyboard)

	payload := map[string]interface{}{
		"chat_id":             chatID,
		"text":                "كيف تصنف هذه الصورة؟ (تحتاج 3 أصوات)",
		"reply_to_message_id": msgID,
		"reply_markup":        string(kbJson),
	}
	
	body, _ := json.Marshal(payload)
	http.Post(url, "application/json", bytes.NewBuffer(body))
}

// دالة معالجة التصويت
func handleVote(up Update, token string) {
	data := up.CallbackQuery.Data // مثال: 12345_food
	parts := strings.Split(data, "_")
	msgID := parts[0]
	tag := parts[1]

	// منطق الحساب (في الإنتاج الحقيقي استخدم Redis أو MongoDB)
	if votes[msgID] == nil {
		votes[msgID] = make(map[string]int)
	}
	votes[msgID][tag]++

	count := votes[msgID][tag]
	
	// إذا وصل الإجماع لـ 3 أصوات (يمكنك زيادتها)
	if count >= 3 {
		announceWinner(up.CallbackQuery.Message.Chat.ID, tag, token)
		delete(votes, msgID) // تنظيف الذاكرة
	} else {
		// الرد على الضغطة (Alert بسيط)
		answerCallback(up.CallbackQuery.ID, fmt.Sprintf("تم تسجيل صوتك لـ %s! (الحالي: %d)", tag, count), token)
	}
}

func announceWinner(chatID int64, tag string, token string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	text := fmt.Sprintf("✅ تم الإجماع! الصورة تم تصنيفها كـ: #%s وسيتم أرشفتها.", tag)
	
	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	body, _ := json.Marshal(payload)
	http.Post(url, "application/json", bytes.NewBuffer(body))
}

func answerCallback(queryID string, text string, token string) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/answerCallbackQuery", token)
	payload := map[string]string{
		"callback_query_id": queryID,
		"text":              text,
	}
	body, _ := json.Marshal(payload)
	http.Post(url, "application/json", bytes.NewBuffer(body))
}
