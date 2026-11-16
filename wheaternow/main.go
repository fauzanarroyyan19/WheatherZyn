package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

// This is the shared WeatherNow API response used by both the website and bot.
type Weather struct {
	City           string         `json:"city"`
	Temperature    float64        `json:"temperature"`
	Status         string         `json:"status"`
	Description    string         `json:"description"`
	Humidity       int            `json:"humidity"`
	WindSpeed      float64        `json:"wind_speed"`
	Icon           string         `json:"icon"`
	Timezone       int            `json:"timezone"`
	Sunrise        int64          `json:"sunrise"`
	Sunset         int64          `json:"sunset"`
	UV             *float64       `json:"uv"`
	Forecast       []DayForecast  `json:"forecast"`
	Hourly         []HourForecast `json:"hourly"`
	Alerts         []WeatherAlert `json:"alerts"`
	OfficialAlerts []WeatherAlert `json:"official_alerts"`
}

type DayForecast struct {
	Date        string  `json:"date"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Description string  `json:"description"`
	Rain        float64 `json:"rain"`
}
type HourForecast struct {
	Time        int64   `json:"time"`
	Temperature float64 `json:"temperature"`
	Description string  `json:"description"`
	Rain        float64 `json:"rain"`
}
type WeatherAlert struct {
	Event       string `json:"event"`
	Description string `json:"description"`
	Start       int64  `json:"start"`
	Source      string `json:"source"`
}
type Subscriber struct {
	ChatID    int64  `json:"chat_id"`
	City      string `json:"city"`
	Offset    int    `json:"offset"`
	DailyTime string `json:"daily_time"`
	LastSent  string `json:"last_sent"`
}
type botState struct {
	Favorites   map[string][]string `json:"favorites"`
	Subscribers []Subscriber        `json:"subscribers"`
}

var stateMu sync.Mutex
var state botState

func main() {
	_ = godotenv.Load()
	loadBotState()
	token := os.Getenv("TELEGRAM_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_TOKEN belum diatur di .env")
	}
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Fatal("Gagal menginisialisasi bot Telegram: ", err)
	}
	log.Printf("WeatherNow Telegram bot aktif: @%s", bot.Self.UserName)
	go runDailyScheduler(bot)

	updates := tgbotapi.NewUpdate(0)
	updates.Timeout = 60
	for update := range bot.GetUpdatesChan(updates) {
		if update.Message != nil {
			switch {
			case update.Message.Text == "/start":
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "🌤 Selamat datang di WeatherNow!\nPilih kota untuk melihat cuaca terkini, atau kirim lokasi Anda.\n\nPerintah: /weather kota, /forecast kota, /favorite add kota, /favorites, /subscribe kota, /unsubscribe")
				msg.Text = welcomeMessage()
				msg.ReplyMarkup = mainMenuKeyboard()
				bot.Send(msg)
				locationKeyboard := tgbotapi.NewReplyKeyboard(tgbotapi.NewKeyboardButtonRow(tgbotapi.NewKeyboardButtonLocation("📍 Kirim lokasi saya")))
				locationKeyboard.OneTimeKeyboard = true
				locationMsg := tgbotapi.NewMessage(update.Message.Chat.ID, "Untuk cuaca berdasarkan lokasi, buka chat pribadi dengan bot ini lalu tekan tombol Kirim lokasi saya.")
				locationMsg.ReplyMarkup = locationKeyboard
				bot.Send(locationMsg)
			case update.Message.Location != nil:
				sendWeatherByCoords(bot, update.Message.Chat.ID, update.Message.Location.Latitude, update.Message.Location.Longitude)
			case strings.TrimSpace(update.Message.Text) != "":
				handleCommand(bot, update.Message.Chat.ID, update.Message.Text)
			}
		}
		if update.CallbackQuery != nil {
			_, _ = bot.Request(tgbotapi.NewCallback(update.CallbackQuery.ID, ""))
			handleCallback(bot, update.CallbackQuery)
		}
	}
}

func fetchWeather(city string) (Weather, error) {
	baseURL := strings.TrimRight(os.Getenv("WEATHER_API_URL"), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	endpoint := baseURL + "/api/weather?city=" + url.QueryEscape(city)
	return fetchWeatherFromAPI(endpoint)
}

func fetchWeatherByCoords(lat, lon float64) (Weather, error) {
	baseURL := strings.TrimRight(os.Getenv("WEATHER_API_URL"), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	endpoint := fmt.Sprintf("%s/api/weather?lat=%f&lon=%f", baseURL, lat, lon)
	return fetchWeatherFromAPI(endpoint)
}

func fetchWeatherFromAPI(endpoint string) (Weather, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		return Weather{}, fmt.Errorf("gagal menghubungi WeatherNow API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var apiError struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&apiError)
		if apiError.Error != "" {
			return Weather{}, fmt.Errorf("%s", apiError.Error)
		}
		return Weather{}, fmt.Errorf("WeatherNow API mengembalikan status %d", resp.StatusCode)
	}
	var weather Weather
	if err := json.NewDecoder(resp.Body).Decode(&weather); err != nil {
		return Weather{}, fmt.Errorf("respons WeatherNow API tidak valid: %w", err)
	}
	return weather, nil
}

func sendWeather(bot *tgbotapi.BotAPI, chatID int64, city string) {
	w, err := fetchWeather(city)
	if err != nil {
		sendText(bot, chatID, "Maaf, cuaca belum bisa dimuat. "+err.Error())
		return
	}
	sendWeatherReport(bot, chatID, w)
}

func sendWeatherByCoords(bot *tgbotapi.BotAPI, chatID int64, lat, lon float64) {
	w, err := fetchWeatherByCoords(lat, lon)
	if err != nil {
		sendText(bot, chatID, "Maaf, cuaca belum bisa dimuat dari lokasi Anda. "+err.Error())
		return
	}
	sendWeatherReport(bot, chatID, w)
}

func sendWeatherReport(bot *tgbotapi.BotAPI, chatID int64, w Weather) {
	localTime := time.Now().UTC().Add(time.Duration(w.Timezone) * time.Second).Format("15:04")
	message := fmt.Sprintf("🌦 *Cuaca di %s*\n🕒 Waktu setempat: %s\n🌡 Suhu: %.1f°C\n💧 Kelembapan: %d%%\n💨 Angin: %.1f m/s\n📝 Kondisi: %s", w.City, localTime, w.Temperature, w.Humidity, w.WindSpeed, w.Description)
	message += fmt.Sprintf("\n🌅 Terbit: %s · 🌇 Terbenam: %s", formatLocal(w.Sunrise, w.Timezone), formatLocal(w.Sunset, w.Timezone))
	if w.UV != nil {
		message += fmt.Sprintf("\n☀️ Indeks UV: %.1f", *w.UV)
	}
	switch {
	case strings.Contains(strings.ToLower(w.Description), "hujan"):
		message += "\n\n☔ Hujan terdeteksi. Bawa payung jika akan bepergian."
	case w.Temperature >= 34:
		message += "\n\n🥵 Cuaca sangat panas. Minum cukup air dan batasi aktivitas di luar."
	case w.Temperature < 20:
		message += "\n\n🧥 Udara dingin. Pertimbangkan membawa jaket."
	}
	for _, alert := range append(w.OfficialAlerts, w.Alerts...) {
		message += fmt.Sprintf("\n\n⚠️ *%s*\n%s", alert.Event, alert.Description)
	}
	msg := tgbotapi.NewMessage(chatID, message)
	msg.ParseMode = "Markdown"
	if _, err := bot.Send(msg); err != nil {
		log.Printf("Gagal mengirim cuaca ke Telegram: %v", err)
	}
}

func formatLocal(unix int64, offset int) string {
	if unix == 0 {
		return "—"
	}
	return time.Unix(unix+int64(offset), 0).UTC().Format("15:04")
}

func welcomeMessage() string {
	return fmt.Sprintf("🌤 Selamat datang di WeatherNow!\n\nPilih menu untuk melihat cuaca, prakiraan, dan menyimpan kota favorit. Kamu juga bisa mengaktifkan ringkasan harian dan memilih jam kirimnya.\n\nPrakiraan 5 hari tersedia tiap 3 jam. Prakiraan per jam, indeks UV, dan peringatan resmi memerlukan akses OpenWeather One Call. Untuk cuaca di sekitar, gunakan tombol Kirim lokasi saya.\n\nJam awal ringkasan: %s waktu kota.", defaultDailyTime())
}

func mainMenuKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("🌦 Cuaca saat ini", "menu:current"), tgbotapi.NewInlineKeyboardButtonData("🗓 Prakiraan", "menu:forecast")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⭐ Kota favorit", "menu:favorites"), tgbotapi.NewInlineKeyboardButtonData("ℹ️ Info fitur", "menu:info")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("🔔 Aktifkan ringkasan", "menu:subscribe"), tgbotapi.NewInlineKeyboardButtonData("🔕 Matikan ringkasan", "menu:unsubscribe")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("⚙️ Atur jam ringkasan", "menu:set_time")),
	)
}

func summaryTimeKeyboard() tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("06.00", "settime:06:00"), tgbotapi.NewInlineKeyboardButtonData("07.00", "settime:07:00"), tgbotapi.NewInlineKeyboardButtonData("08.00", "settime:08:00")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("09.00", "settime:09:00"), tgbotapi.NewInlineKeyboardButtonData("18.00", "settime:18:00"), tgbotapi.NewInlineKeyboardButtonData("20.00", "settime:20:00")),
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("‹ Menu utama", "menu:home")),
	)
}

func cityPickerKeyboard(action string, chatID int64) tgbotapi.InlineKeyboardMarkup {
	cities := []string{"Jakarta", "Bandung", "Bekasi", "Surabaya", "Denpasar", "Makassar"}
	stateMu.Lock()
	for _, city := range state.Favorites[fmt.Sprint(chatID)] {
		found := false
		for _, known := range cities {
			if strings.EqualFold(city, known) {
				found = true
				break
			}
		}
		if !found {
			cities = append(cities, city)
		}
	}
	stateMu.Unlock()
	rows := [][]tgbotapi.InlineKeyboardButton{}
	for i := 0; i < len(cities); i += 2 {
		row := []tgbotapi.InlineKeyboardButton{tgbotapi.NewInlineKeyboardButtonData(cities[i], action+":"+cities[i])}
		if i+1 < len(cities) {
			row = append(row, tgbotapi.NewInlineKeyboardButtonData(cities[i+1], action+":"+cities[i+1]))
		}
		rows = append(rows, row)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("‹ Menu utama", "menu:home")))
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func favoritesKeyboard(chatID int64) (string, tgbotapi.InlineKeyboardMarkup) {
	stateMu.Lock()
	cities := append([]string(nil), state.Favorites[fmt.Sprint(chatID)]...)
	stateMu.Unlock()
	rows := [][]tgbotapi.InlineKeyboardButton{}
	for _, city := range cities {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("🌦 "+city, "weather:"+city)))
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("➕ Tambah favorit", "menu:favorite_add"), tgbotapi.NewInlineKeyboardButtonData("➖ Hapus favorit", "menu:favorite_remove")))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData("‹ Menu utama", "menu:home")))
	if len(cities) == 0 {
		return "Belum ada kota favorit. Tekan Tambah favorit untuk menyimpan kota.", tgbotapi.NewInlineKeyboardMarkup(rows...)
	}
	return "Pilih kota favorit untuk melihat cuacanya.", tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func handleCallback(bot *tgbotapi.BotAPI, query *tgbotapi.CallbackQuery) {
	if query.Message == nil {
		return
	}
	chatID := query.Message.Chat.ID
	parts := strings.SplitN(query.Data, ":", 2)
	if len(parts) == 1 {
		sendWeather(bot, chatID, query.Data)
		return
	}
	action, value := parts[0], parts[1]
	switch action {
	case "menu":
		switch value {
		case "home":
			msg := tgbotapi.NewMessage(chatID, welcomeMessage())
			msg.ReplyMarkup = mainMenuKeyboard()
			bot.Send(msg)
		case "current", "forecast", "subscribe", "favorite_add", "favorite_remove":
			prompt := "Pilih kota:"
			if value == "forecast" {
				prompt = "Pilih kota untuk melihat prakiraan:"
			}
			if value == "subscribe" {
				prompt = "Pilih kota untuk ringkasan harian. Jamnya bisa diatur setelah aktif:"
			}
			if value == "favorite_add" {
				prompt = "Pilih kota untuk menambah ke favorit:"
			}
			if value == "favorite_remove" {
				prompt = "Pilih kota favorit yang ingin dihapus:"
			}
			msg := tgbotapi.NewMessage(chatID, prompt)
			action := value
			if action == "current" {
				action = "weather"
			}
			msg.ReplyMarkup = cityPickerKeyboard(action, chatID)
			bot.Send(msg)
		case "favorites":
			text, keyboard := favoritesKeyboard(chatID)
			msg := tgbotapi.NewMessage(chatID, text)
			msg.ReplyMarkup = keyboard
			bot.Send(msg)
		case "unsubscribe":
			unsubscribe(chatID)
			sendText(bot, chatID, "Ringkasan harian dinonaktifkan.")
		case "set_time":
			msg := tgbotapi.NewMessage(chatID, "Pilih jam kirim ringkasan. Jam mengikuti zona waktu kota yang Anda langgani.")
			msg.ReplyMarkup = summaryTimeKeyboard()
			bot.Send(msg)
		case "info":
			msg := tgbotapi.NewMessage(chatID, "🌦 Cuaca saat ini: suhu, kondisi, angin, kelembapan, dan waktu matahari.\n🗓 Prakiraan 5 hari: data per 3 jam.\n⭐ Favorit: simpan kota di akun Telegram ini.\n🔔 Ringkasan: kiriman otomatis pada jam pilihan menurut zona waktu kota. Bot harus terus berjalan.\n☀️ Prakiraan per jam, indeks UV, dan peringatan resmi membutuhkan akses One Call OpenWeather.")
			msg.ReplyMarkup = mainMenuKeyboard()
			bot.Send(msg)
		}
	case "weather":
		sendWeather(bot, chatID, value)
	case "forecast":
		sendForecast(bot, chatID, value)
	case "subscribe":
		w, err := fetchWeather(value)
		if err != nil {
			sendText(bot, chatID, "Ringkasan tidak bisa diaktifkan: "+err.Error())
			return
		}
		subscribe(chatID, w.City, w.Timezone)
		sendText(bot, chatID, "🔔 Ringkasan harian untuk "+w.City+" aktif pukul "+defaultDailyTime()+" waktu setempat. Gunakan Atur jam ringkasan untuk mengubah jadwal.")
	case "favadd":
		updateFavorite(chatID, value, true)
		sendText(bot, chatID, value+" ditambahkan ke kota favorit.")
	case "favrm":
		updateFavorite(chatID, value, false)
		sendText(bot, chatID, value+" dihapus dari kota favorit.")
	case "settime":
		if _, err := time.Parse("15:04", value); err != nil {
			sendText(bot, chatID, "Format jam tidak valid.")
			return
		}
		if !setSubscriberTime(chatID, value) {
			sendText(bot, chatID, "Aktifkan ringkasan harian dulu lewat menu Aktifkan ringkasan.")
			return
		}
		sendText(bot, chatID, "⏰ Jam ringkasan diubah menjadi "+value+" waktu kota pilihan.")
	}
}

func handleCommand(bot *tgbotapi.BotAPI, chatID int64, text string) {
	parts := strings.Fields(text)
	if len(parts) == 0 {
		return
	}
	command := strings.ToLower(strings.SplitN(parts[0], "@", 2)[0])
	args := strings.TrimSpace(strings.TrimPrefix(text, parts[0]))
	switch command {
	case "/weather":
		if args == "" {
			sendText(bot, chatID, "Contoh: /weather Jakarta")
			return
		}
		sendWeather(bot, chatID, args)
	case "/forecast":
		if args == "" {
			sendText(bot, chatID, "Contoh: /forecast Jakarta")
			return
		}
		sendForecast(bot, chatID, args)
	case "/favorite":
		fields := strings.Fields(args)
		if len(fields) < 2 || (fields[0] != "add" && fields[0] != "remove") {
			sendText(bot, chatID, "Gunakan /favorite add Jakarta atau /favorite remove Jakarta")
			return
		}
		city := strings.TrimSpace(strings.TrimPrefix(args, fields[0]))
		updateFavorite(chatID, city, fields[0] == "add")
		sendText(bot, chatID, "Favorit diperbarui. Ketik /favorites untuk melihat daftar.")
	case "/favorites":
		message, keyboard := favoritesKeyboard(chatID)
		msg := tgbotapi.NewMessage(chatID, message)
		msg.ReplyMarkup = keyboard
		bot.Send(msg)
	case "/subscribe":
		if args == "" {
			sendText(bot, chatID, "Pilih kota untuk ringkasan harian: /subscribe Jakarta")
			return
		}
		w, err := fetchWeather(args)
		if err != nil {
			sendText(bot, chatID, "Tidak bisa mengaktifkan ringkasan: "+err.Error())
			return
		}
		subscribe(chatID, w.City, w.Timezone)
		sendText(bot, chatID, "🔔 Ringkasan harian untuk "+w.City+" aktif pukul "+defaultDailyTime()+" waktu setempat. Gunakan /time 07:30 untuk mengubah jam atau /unsubscribe untuk berhenti.")
	case "/unsubscribe":
		unsubscribe(chatID)
		sendText(bot, chatID, "Ringkasan harian dinonaktifkan.")
	case "/time":
		if _, err := time.Parse("15:04", args); err != nil {
			sendText(bot, chatID, "Contoh format: /time 07:30")
			return
		}
		if !setSubscriberTime(chatID, args) {
			sendText(bot, chatID, "Aktifkan ringkasan harian dulu dengan /subscribe Jakarta.")
			return
		}
		sendText(bot, chatID, "Jam ringkasan diubah menjadi "+args+" waktu kota pilihan.")
	case "/start", "/help":
		msg := tgbotapi.NewMessage(chatID, welcomeMessage())
		msg.ReplyMarkup = mainMenuKeyboard()
		bot.Send(msg)
	}
}

func sendForecast(bot *tgbotapi.BotAPI, chatID int64, city string) {
	w, err := fetchWeather(city)
	if err != nil {
		sendText(bot, chatID, "Prakiraan belum bisa dimuat: "+err.Error())
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🌤 *Prakiraan %s*\n", w.City)
	for index, d := range w.Forecast {
		if index >= 5 {
			break
		}
		fmt.Fprintf(&b, "• %s: %s, %.0f°–%.0f°", d.Date, d.Description, d.Min, d.Max)
		if d.Rain > 0 {
			fmt.Fprintf(&b, ", peluang hujan %.0f%%", d.Rain*100)
		}
		b.WriteByte('\n')
	}
	if len(w.Hourly) > 0 {
		b.WriteString("\n*Beberapa jam ke depan*\n")
		limit := 6
		if len(w.Hourly) < limit {
			limit = len(w.Hourly)
		}
		for _, h := range w.Hourly[:limit] {
			fmt.Fprintf(&b, "• %s · %.0f°C · %s · hujan %.0f%%\n", formatLocal(h.Time, w.Timezone), h.Temperature, h.Description, h.Rain*100)
		}
	}
	if w.UV != nil {
		fmt.Fprintf(&b, "\nIndeks UV saat ini: %.1f", *w.UV)
	}
	for _, a := range append(w.OfficialAlerts, w.Alerts...) {
		fmt.Fprintf(&b, "\n\n⚠️ *%s*\n%s", a.Event, a.Description)
	}
	msg := tgbotapi.NewMessage(chatID, b.String())
	msg.ParseMode = "Markdown"
	if _, err := bot.Send(msg); err != nil {
		log.Printf("gagal mengirim prakiraan: %v", err)
	}
}

func statePath() string {
	if path := strings.TrimSpace(os.Getenv("BOT_STATE_FILE")); path != "" {
		return path
	}
	return "bot_state.json"
}
func loadBotState() {
	state = botState{Favorites: map[string][]string{}}
	data, err := os.ReadFile(statePath())
	if err == nil {
		_ = json.Unmarshal(data, &state)
	}
	if state.Favorites == nil {
		state.Favorites = map[string][]string{}
	}
}
func saveBotStateLocked() {
	data, err := json.MarshalIndent(state, "", "  ")
	if err == nil {
		if err = os.WriteFile(statePath(), data, 0600); err != nil {
			log.Printf("gagal menyimpan preferensi bot: %v", err)
		}
	}
}
func updateFavorite(chatID int64, city string, add bool) {
	if city == "" {
		return
	}
	key := fmt.Sprint(chatID)
	stateMu.Lock()
	defer stateMu.Unlock()
	items := state.Favorites[key]
	found := false
	next := []string{}
	for _, item := range items {
		if strings.EqualFold(item, city) {
			found = true
			if !add {
				continue
			}
		}
		next = append(next, item)
	}
	if add && !found {
		next = append(next, city)
	}
	state.Favorites[key] = next
	saveBotStateLocked()
}
func subscribe(chatID int64, city string, offset int) {
	stateMu.Lock()
	defer stateMu.Unlock()
	for i := range state.Subscribers {
		if state.Subscribers[i].ChatID == chatID {
			state.Subscribers[i].City = city
			state.Subscribers[i].Offset = offset
			state.Subscribers[i].DailyTime = defaultDailyTime()
			state.Subscribers[i].LastSent = ""
			saveBotStateLocked()
			return
		}
	}
	state.Subscribers = append(state.Subscribers, Subscriber{ChatID: chatID, City: city, Offset: offset, DailyTime: defaultDailyTime()})
	saveBotStateLocked()
}
func setSubscriberTime(chatID int64, dailyTime string) bool {
	stateMu.Lock()
	defer stateMu.Unlock()
	for i := range state.Subscribers {
		if state.Subscribers[i].ChatID == chatID {
			state.Subscribers[i].DailyTime = dailyTime
			state.Subscribers[i].LastSent = ""
			saveBotStateLocked()
			return true
		}
	}
	return false
}
func defaultDailyTime() string {
	configured := strings.TrimSpace(os.Getenv("DAILY_SUMMARY_TIME"))
	if _, err := time.Parse("15:04", configured); err == nil {
		return configured
	}
	return "08:00"
}
func unsubscribe(chatID int64) {
	stateMu.Lock()
	defer stateMu.Unlock()
	next := []Subscriber{}
	for _, sub := range state.Subscribers {
		if sub.ChatID != chatID {
			next = append(next, sub)
		}
	}
	state.Subscribers = next
	saveBotStateLocked()
}
func runDailyScheduler(bot *tgbotapi.BotAPI) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		stateMu.Lock()
		subs := append([]Subscriber(nil), state.Subscribers...)
		stateMu.Unlock()
		for _, sub := range subs {
			summaryTime := sub.DailyTime
			if _, err := time.Parse("15:04", summaryTime); err != nil {
				summaryTime = defaultDailyTime()
			}
			parsedTime, _ := time.Parse("15:04", summaryTime)
			hour, minute := parsedTime.Hour(), parsedTime.Minute()
			local := time.Now().UTC().Add(time.Duration(sub.Offset) * time.Second)
			day := local.Format("2006-01-02")
			if local.Hour() != hour || local.Minute() != minute || sub.LastSent == day {
				continue
			}
			w, err := fetchWeather(sub.City)
			if err != nil {
				sendText(bot, sub.ChatID, "Ringkasan harian gagal dimuat: "+err.Error())
				continue
			}
			sendWeatherReport(bot, sub.ChatID, w)
			stateMu.Lock()
			for i := range state.Subscribers {
				if state.Subscribers[i].ChatID == sub.ChatID {
					state.Subscribers[i].LastSent = day
				}
			}
			saveBotStateLocked()
			stateMu.Unlock()
		}
	}
}

func sendText(bot *tgbotapi.BotAPI, chatID int64, text string) {
	if _, err := bot.Send(tgbotapi.NewMessage(chatID, text)); err != nil {
		log.Printf("Gagal mengirim pesan Telegram: %v", err)
	}
}

func cityKeyboard() tgbotapi.InlineKeyboardMarkup {
	rows := [][]tgbotapi.InlineKeyboardButton{
		{tgbotapi.NewInlineKeyboardButtonData("🏙 Jakarta", "Jakarta"), tgbotapi.NewInlineKeyboardButtonData("🌆 Bandung", "Bandung")},
		{tgbotapi.NewInlineKeyboardButtonData("🌊 Surabaya", "Surabaya"), tgbotapi.NewInlineKeyboardButtonData("🌴 Denpasar", "Denpasar")},
		{tgbotapi.NewInlineKeyboardButtonData("🌇 Bekasi", "Bekasi"), tgbotapi.NewInlineKeyboardButtonData("🏝 Makassar", "Makassar")},
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}
