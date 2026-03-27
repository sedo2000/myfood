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

// هيكل البيانات المطور
type MemberStats struct {
	Reputation int       `json:"reputation"`
	LastMsg    time.Time `json:"last_msg"`
	MsgCount   int       `json:"msg_count"`
	JoinDate   time.Time `json:"join_date"`
}

var (
	db    = make(map[int64]*MemberStats)
	mu    sync.Mutex
	token = os.Getenv("TELEGRAM_TOKEN")
)

type Update struct {
	Message struct {
		MessageID int64 `json:"message_id"`
		Text      string `json:"text"`
		Chat      struct{ ID int64 } `json:"chat"`
		From      struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
			Username  string `json:"username"`
		} `json:"from"`
		ReplyToMessage *struct {
			From struct {
				ID        int64  `json:"id"`
				FirstName string `json:"first_name"`
			} `json:"from"`
		} `json:"reply_to_message"`
	} `json:"message"`
}

func Handler(w http.ResponseWriter, r *http.Request) {
	var update Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		return
	}

	if update.Message.Text != "" {
		go handleLogic(update)
	}

	w.WriteHeader(http.StatusOK)
}

func handleLogic(up Update) {
	text := up.Message.Text
	chatID := up.Message.Chat.ID
	userID := up.Message.From.ID

	// تسجيل العضو إذا كان جديداً
	mu.Lock()
	stats, exists := db[userID]
	if !exists {
		stats = &MemberStats{Reputation: 50, JoinDate: time.Now(), LastMsg: time.Now()}
		db[userID] = stats
	}
	mu.Unlock()

	// 1. معالجة الأوامر (Commands)
	if strings.HasPrefix(text, "/") {
		handleCommands(up, stats)
		return
	}

	// 2. نظام الحماية التلقائي (الذي كتبناه سابقاً)
	processAutoMod(up, stats)
}

func handleCommands(up Update, stats *MemberStats) {
	text := up.Message.Text
	chatID := up.Message.Chat.ID
	userID := up.Message.From.ID

	switch {
	// أمر عرض الملف الشخصي والسمعة
	case strings.HasPrefix(text, "/me"):
		msg := fmt.Sprintf("👤 **الملف الشخصي لـ %s**\n\n🏅 السمعة: `%d/100`\n📅 انضممت: %s\n🛡️ الحالة: %s", 
			up.Message.From.FirstName, stats.Reputation, stats.JoinDate.Format("2006-01-02"), getStatus(stats.Reputation))
		sendResponse(chatID, msg, up.Message.MessageID)

	// أمر إداري: طرد (يجب استخدامه بالرد على رسالة)
	case strings.HasPrefix(text, "/ban") && up.Message.ReplyToMessage != nil:
		targetID := up.Message.ReplyToMessage.From.ID
		banUser(chatID, targetID)
		sendResponse(chatID, "🚫 تم طرد العضو بنجاح.", up.Message.MessageID)

	// أمر إداري: تحذير وتنقيص سمعة
	case strings.HasPrefix(text, "/warn") && up.Message.ReplyToMessage != nil:
		targetID := up.Message.ReplyToMessage.From.ID
		mu.Lock()
		if targetStats, ok := db[targetID]; ok {
			targetStats.Reputation -= 15
			msg := fmt.Sprintf("⚠️ تم تحذير [%s](tg://user?id=%d).\nالسمعة الحالية: %d", 
				up.Message.ReplyToMessage.From.FirstName, targetID, targetStats.Reputation)
			sendResponse(chatID, msg, up.Message.MessageID)
			if targetStats.Reputation <= 0 { banUser(chatID, targetID) }
		}
		mu.Unlock()

	// أمر عرض القوانين
	case strings.HasPrefix(text, "/rules"):
		rules := "📜 **قوانين المجموعة لعام 2026:**\n1. لا سبام.\n2. الاحترام المتبادل.\n3. السمعة المنخفضة تؤدي للطرد التلقائي."
		sendResponse(chatID, rules, up.Message.MessageID)
	}
}

// دالة لتحديد حالة العضو بناءً على نقاطه
func getStatus(rep int) string {
	if rep >= 80 { return "عضو ذهبي ✨" }
	if rep >= 50 { return "عضو نشط ✅" }
	if rep >= 20 { return "تحت المراقبة ⚠️" }
	return "خطر (سيتم طردك) ⛔"
}

// --- الدوال المساعدة لإرسال الطلبات ---

func sendResponse(chatID int64, text string, replyID int64) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", token)
	payload := map[string]interface{}{
		"chat_id":             chatID,
		"text":                text,
		"reply_to_message_id": replyID,
		"parse_mode":          "Markdown",
	}
	sendRequest(url, payload)
}

func processAutoMod(up Update, stats *MemberStats) {
	// هنا نضع منطق الـ isMalicious والـ Rate Limit الذي كتبناه في الرد السابق
}

func banUser(chatID, userID int64) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/banChatMember", token)
	payload := map[string]interface{}{"chat_id": chatID, "user_id": userID}
	sendRequest(url, payload)
}

func sendRequest(url string, payload interface{}) {
	b, _ := json.Marshal(payload)
	http.Post(url, "application/json", bytes.NewBuffer(b))
}
