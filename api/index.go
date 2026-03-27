package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var token = os.Getenv("TELEGRAM_TOKEN")

// --- قواعد البيانات المؤقتة (يجب استبدالها بـ MongoDB أو Redis لاحقاً) ---
var (
	groupSettings = make(map[int64]*Settings) // إعدادات القفل والفتح لكل مجموعة
	userRanks     = make(map[string]string)   // رتب المستخدمين (مفتاح: ChatID_UserID)
	bankAccounts  = make(map[int64]*Bank)     // أرصدة لعبة البنك
	mu            sync.Mutex
)

type Settings struct {
	LockLinks   bool
	LockPhotos  bool
	LockStickers bool
}

type Bank struct {
	Balance    int64
	LastSalary time.Time
}

// --- هياكل بيانات تليجرام ---
type Update struct {
	Message struct {
		MessageID int64  `json:"message_id"`
		Text      string `json:"text"`
		Chat      struct{ ID int64 } `json:"chat"`
		From      struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
		} `json:"from"`
		ReplyToMessage *struct {
			MessageID int64 `json:"message_id"`
			From      struct {
				ID        int64  `json:"id"`
				FirstName string `json:"first_name"`
			} `json:"from"`
		} `json:"reply_to_message"`
		Entities []struct {
			Type string `json:"type"`
		} `json:"entities"`
	} `json:"message"`
	CallbackQuery struct {
		Data string `json:"data"`
		Message struct {
			MessageID int64 `json:"message_id"`
			Chat      struct{ ID int64 } `json:"chat"`
		} `json:"message"`
	} `json:"callback_query"`
}

// --- الدالة الرئيسية (نقطة الدخول لـ Vercel) ---
func Handler(w http.ResponseWriter, r *http.Request) {
	var update Update
	json.NewDecoder(r.Body).Decode(&update)

	if update.CallbackQuery.Data != "" {
		handleCallbacks(update)
		w.WriteHeader(http.StatusOK)
		return
	}

	if update.Message.Text != "" || len(update.Message.Entities) > 0 {
		go processMessage(update) // تشغيل المعالجة في الخلفية للسرعة
	}

	w.WriteHeader(http.StatusOK)
}

// --- معالجة الرسائل والأوامر ---
func processMessage(up Update) {
	chatID := up.Message.Chat.ID
	userID := up.Message.From.ID
	text := up.Message.Text
	msgID := up.Message.MessageID

	// 1. فحص الإعدادات للمجموعة (إنشاء إعدادات افتراضية إذا لم تكن موجودة)
	mu.Lock()
	if _, exists := groupSettings[chatID]; !exists {
		groupSettings[chatID] = &Settings{}
	}
	settings := groupSettings[chatID]
	mu.Unlock()

	// 2. نظام الحماية (الحذف التلقائي)
	if isViolation(up, settings) {
		deleteMessage(chatID, msgID)
		return // إذا كانت الرسالة مخالفة وتم حذفها، لا تنفذ أي أوامر
	}

	// 3. معالجة أوامر القفل والفتح (للمشرفين فقط - يمكن إضافة شرط الرتبة لاحقاً)
	if strings.HasPrefix(text, "قفل ") || strings.HasPrefix(text, "فتح ") {
		handleLocks(chatID, text, settings, msgID)
		return
	}

	// 4. معالجة أوامر الرفع والتنزيل (بالرد)
	if (strings.HasPrefix(text, "رفع ") || strings.HasPrefix(text, "تنزيل ")) && up.Message.ReplyToMessage != nil {
		handleRanks(chatID, text, up.Message.ReplyToMessage.From.ID, up.Message.ReplyToMessage.From.FirstName, msgID)
		return
	}

	// 5. أوامر الطرد والحظر
	if (text == "حظر" || text == "طرد" || text == "كتم") && up.Message.ReplyToMessage != nil {
		handlePunishments(chatID, text, up.Message.ReplyToMessage.From.ID, msgID)
		return
	}

	// 6. لعبة البنك والاقتصاد
	if text == "راتب" || text == "رصيدي" {
		handleBank(chatID, userID, text, msgID)
		return
	}

	// 7. القائمة الرئيسية
	if text == "الاوامر" || text == "مساعدة" {
		sendMainMenu(chatID)
	}
}

// --- نظام القفل والفتح ---
func handleLocks(chatID int64, text string, settings *Settings, msgID int64) {
	action := "🔒 تم قفل"
	state := true
	if strings.HasPrefix(text, "فتح ") {
		action = "🔓 تم فتح"
		state = false
	}

	mu.Lock()
	defer mu.Unlock()
	reply := ""

	if strings.Contains(text, "الروابط") {
		settings.LockLinks = state
		reply = action + " الروابط بنجاح."
	} else if strings.Contains(text, "الصور") {
		settings.LockPhotos = state
		reply = action + " الصور بنجاح."
	} else if strings.Contains(text, "الملصقات") {
		settings.LockStickers = state
		reply = action + " الملصقات بنجاح."
	}

	if reply != "" {
		sendText(chatID, reply, msgID)
	}
}

// دالة فحص المخالفات
func isViolation(up Update, settings *Settings) bool {
	// فحص الروابط
	if settings.LockLinks {
		for _, ent := range up.Message.Entities {
			if ent.Type == "url" || ent.Type == "text_link" {
				return true
			}
		}
	}
	return false
}

// --- نظام الرتب (رفع وتنزيل) ---
func handleRanks(chatID int64, command string, targetID int64, targetName string, msgID int64) {
	key := fmt.Sprintf("%d_%d", chatID, targetID)
	mu.Lock()
	defer mu.Unlock()

	var reply string
	if command == "رفع مدير" {
		userRanks[key] = "مدير"
		reply = fmt.Sprintf("👤 العضو: [%s](tg://user?id=%d)\n📊 تم رفعه ليصبح **مدير** في المجموعة.", targetName, targetID)
	} else if command == "تنزيل مدير" {
		delete(userRanks, key)
		reply = fmt.Sprintf("👤 العضو: [%s](tg://user?id=%d)\n🔻 تم تنزيله من رتبة مدير.", targetName, targetID)
	}
	// يمكنك نسخ هذا المنطق لباقي الرتب (أدمن، مميز، مالك)
	
	if reply != "" {
		sendText(chatID, reply, msgID)
	}
}

// --- نظام العقوبات ---
func handlePunishments(chatID int64, command string, targetID int64, msgID int64) {
	if command == "حظر" || command == "طرد" {
		url := fmt.Sprintf("https://api.telegram.org/bot%s/banChatMember", token)
		sendRequest(url, map[string]interface{}{"chat_id": chatID, "user_id": targetID})
		sendText(chatID, "🚫 تم حظر العضو بنجاح.", msgID)
	}
	// للمزيد: استخدم restrictChatMember للكتم
}

// --- نظام البنك الاقتصادي ---
func handleBank(chatID, userID int64, command string, msgID int64) {
	mu.Lock()
	if _, exists := bankAccounts[userID]; !exists {
		bankAccounts[userID] = &Bank{Balance: 0, LastSalary: time.Now().Add(-24 * time.Hour)}
	}
	acc := bankAccounts[userID]
	mu.Unlock()

	if command == "راتب" {
		if time.Since(acc.LastSalary).Hours() >= 24 {
			acc.Balance += 5000 // إيداع الراتب
			acc.LastSalary = time.Now()
			sendText(chatID, "💰 تم إيداع راتبك اليومي بقيمة **5000 دولار** في حسابك.", msgID)
		} else {
			hoursLeft := 24 - time.Since(acc.LastSalary).Hours()
			sendText(chatID, fmt.Sprintf("⏳ لقد استلمت راتبك مسبقاً! انتظر %.1f ساعة.", hoursLeft), msgID)
		}
	} else if command == "رصيدي" {
		sendText(chatID, fmt.Sprintf("💳 رصيدك الحالي في البنك هو: **%d دولار**", acc.Balance), msgID)
	}
}

// --- واجهة الأزرار التفاعلية ---
func sendMainMenu(chatID int64) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	keyboard := map[string]interface{}{
		"inline_keyboard": [][]map[string]interface{}{
			{{ "text": "قائمة م1 🛠️", "callback_data": "m1" }, { "text": "قائمة م2 ⚙️", "callback_data": "m2" }},
			{{ "text": "قائمة م3 📝", "callback_data": "m3" }},
			{{ "text": "الألعاب والبنك 🎮💰", "callback_data": "games" }},
		},
	}
	sendRequest(url, map[string]interface{}{"chat_id": chatID, "text": "🛡️ **أهلاً بك في سورس الحماية**\nإختر من القوائم أدناه:", "reply_markup": keyboard})
}

func handleCallbacks(up Update) {
	chatID := up.CallbackQuery.Message.Chat.ID
	msgID := up.CallbackQuery.Message.MessageID
	
	var text string
	switch up.CallbackQuery.Data {
	case "m1": text = "❨ أوامر الرفع والتنزيل ❩\n⌯ رفع/تنزيل مدير\n⌯ مسح الردود\n⌯ حظر بالرد\n⌯ طرد بالرد"
	case "m2": text = "❨ أوامر القفل والفتح ❩\n⌯ قفل/فتح الروابط\n⌯ قفل/فتح الصور\n⌯ قفل/فتح الملصقات"
	case "games": text = "💰 **لعبة البنك**\n\nأرسل `راتب` لاستلام أموالك\nأرسل `رصيدي` لمعرفة حسابك"
	}

	editUrl := fmt.Sprintf("https://api.telegram.org/bot%s/editMessageText", token)
	sendRequest(editUrl, map[string]interface{}{"chat_id": chatID, "message_id": msgID, "text": text})
}

// --- دوال مساعدة لـ Telegram API ---
func sendText(chatID int64, text string, replyID int64) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	sendRequest(url, map[string]interface{}{"chat_id": chatID, "text": text, "reply_to_message_id": replyID, "parse_mode": "Markdown"})
}

func deleteMessage(chatID, msgID int64) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/deleteMessage", token)
	sendRequest(url, map[string]interface{}{"chat_id": chatID, "message_id": msgID})
}

func sendRequest(url string, payload interface{}) {
	b, _ := json.Marshal(payload)
	http.Post(url, "application/json", bytes.NewBuffer(b))
}
