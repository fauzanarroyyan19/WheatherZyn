package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

type Weather struct {
	City           string           `json:"city"`
	Temperature    float64          `json:"temperature"`
	FeelsLike      float64          `json:"feels_like"`
	Status         string           `json:"status"`
	Description    string           `json:"description"`
	Humidity       int              `json:"humidity"`
	WindSpeed      float64          `json:"wind_speed"`
	Icon           string           `json:"icon"`
	Timezone       int              `json:"timezone"`
	Sunrise        int64            `json:"sunrise"`
	Sunset         int64            `json:"sunset"`
	UV             *float64         `json:"uv"`
	Forecast       []DailyForecast  `json:"forecast"`
	Hourly         []HourlyForecast `json:"hourly"`
	Alerts         []WeatherAlert   `json:"alerts"`
	OfficialAlerts []WeatherAlert   `json:"official_alerts"`
	ForecastSource string           `json:"forecast_source"`
}
type DailyForecast struct {
	Date        string  `json:"date"`
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Description string  `json:"description"`
	Icon        string  `json:"icon"`
	Rain        float64 `json:"rain"`
}
type HourlyForecast struct {
	Time        int64   `json:"time"`
	Temperature float64 `json:"temperature"`
	Description string  `json:"description"`
	Icon        string  `json:"icon"`
	Rain        float64 `json:"rain"`
}
type WeatherAlert struct {
	Event       string `json:"event"`
	Description string `json:"description"`
	Start       int64  `json:"start"`
	End         int64  `json:"end"`
	Source      string `json:"source"`
}
type HistoryPoint struct {
	City        string  `json:"city"`
	Time        int64   `json:"time"`
	Temperature float64 `json:"temperature"`
}

type currentAPI struct {
	Name  string `json:"name"`
	Coord struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"coord"`
	Main struct {
		Temp      float64 `json:"temp"`
		FeelsLike float64 `json:"feels_like"`
		Humidity  int     `json:"humidity"`
	} `json:"main"`
	Weather []struct {
		Description string `json:"description"`
		Main        string `json:"main"`
		Icon        string `json:"icon"`
	} `json:"weather"`
	Wind struct {
		Speed float64 `json:"speed"`
	} `json:"wind"`
	Timezone int `json:"timezone"`
	Sys      struct {
		Sunrise int64 `json:"sunrise"`
		Sunset  int64 `json:"sunset"`
	} `json:"sys"`
}
type forecastAPI struct {
	List []struct {
		DT   int64 `json:"dt"`
		Main struct {
			Temp float64 `json:"temp"`
		} `json:"main"`
		Weather []struct {
			Description string `json:"description"`
			Icon        string `json:"icon"`
		} `json:"weather"`
		Pop float64 `json:"pop"`
	} `json:"list"`
}
type oneCallAPI struct {
	TimezoneOffset int `json:"timezone_offset"`
	Current        struct {
		UVI     float64 `json:"uvi"`
		Sunrise int64   `json:"sunrise"`
		Sunset  int64   `json:"sunset"`
	} `json:"current"`
	Hourly []struct {
		DT      int64   `json:"dt"`
		Temp    float64 `json:"temp"`
		Weather []struct {
			Description string `json:"description"`
			Icon        string `json:"icon"`
		} `json:"weather"`
		Pop float64 `json:"pop"`
	} `json:"hourly"`
	Daily []struct {
		DT   int64 `json:"dt"`
		Temp struct {
			Min float64 `json:"min"`
			Max float64 `json:"max"`
		} `json:"temp"`
		Weather []struct {
			Description string `json:"description"`
			Icon        string `json:"icon"`
		} `json:"weather"`
		Pop float64 `json:"pop"`
	} `json:"daily"`
	Alerts []struct {
		Event       string `json:"event"`
		Description string `json:"description"`
		Start       int64  `json:"start"`
		End         int64  `json:"end"`
		Sender      string `json:"sender_name"`
	} `json:"alerts"`
}

var historyMu sync.Mutex
var history []HistoryPoint
var cacheMu sync.Mutex
var weatherCache = map[string]cachedWeather{}
var rateMu sync.Mutex
var rateWindows = map[string]rateWindow{}

type cachedWeather struct {
	data    Weather
	expires time.Time
}
type rateWindow struct {
	minute int64
	count  int
}
type upstreamError struct {
	status  int
	message string
}

func (e *upstreamError) Error() string { return e.message }

func main() {
	if err := godotenv.Load(); err != nil {
		_ = godotenv.Load("../.env")
	}
	loadHistory()
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"service": "WeatherNow API", "status": "ok", "endpoint": "/api/weather?city=jakarta"})
	})
	mux.Handle("/api/weather", limitRequests(http.HandlerFunc(getWeather)))
	mux.Handle("/api/history", limitRequests(http.HandlerFunc(getHistory)))
	server := &http.Server{Addr: ":" + port, Handler: cors(mux), ReadHeaderTimeout: 5 * time.Second}
	fmt.Printf("WeatherNow API berjalan di http://localhost:%s\n", port)
	log.Fatal(server.ListenAndServe())
}

func getWeather(w http.ResponseWriter, r *http.Request) {
	lang := weatherLanguage(r.URL.Query().Get("lang"))
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": localized(lang, "method")})
		return
	}
	city := strings.TrimSpace(r.URL.Query().Get("city"))
	latText, lonText := strings.TrimSpace(r.URL.Query().Get("lat")), strings.TrimSpace(r.URL.Query().Get("lon"))
	byCoords := latText != "" || lonText != ""
	if !byCoords && city == "" {
		writeJSON(w, 400, map[string]string{"error": localized(lang, "city_required")})
		return
	}
	if byCoords && (latText == "" || lonText == "") {
		writeJSON(w, 400, map[string]string{"error": localized(lang, "coordinates_required")})
		return
	}
	key := strings.TrimSpace(os.Getenv("OPENWEATHER_API_KEY"))
	if key == "" {
		key = strings.TrimSpace(os.Getenv("API_KEY"))
	}
	if key == "" {
		writeJSON(w, 503, map[string]string{"error": localized(lang, "missing_key")})
		return
	}
	query := "q=" + url.QueryEscape(city)
	lat, lon := 0.0, 0.0
	if byCoords {
		var errLat, errLon error
		lat, errLat = strconv.ParseFloat(latText, 64)
		lon, errLon = strconv.ParseFloat(lonText, 64)
		if errLat != nil || errLon != nil || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			writeJSON(w, 400, map[string]string{"error": localized(lang, "invalid_coordinates")})
			return
		}
		query = fmt.Sprintf("lat=%f&lon=%f", lat, lon)
	}
	cacheKey := query + ":lang=" + lang + ":onecall=" + strings.ToLower(os.Getenv("OPENWEATHER_ONECALL_ENABLED"))
	if data, ok := getCachedWeather(cacheKey); ok {
		recordHistory(HistoryPoint{City: data.City, Time: time.Now().Unix(), Temperature: data.Temperature})
		writeJSON(w, 200, data)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var current currentAPI
	if err := fetchJSON(ctx, "https://api.openweathermap.org/data/2.5/weather?"+query+"&units=metric&lang="+lang+"&appid="+url.QueryEscape(key), &current); err != nil {
		status, message := publicWeatherError(err, lang)
		writeJSON(w, status, map[string]string{"error": message})
		return
	}
	if len(current.Weather) == 0 {
		writeJSON(w, 502, map[string]string{"error": localized(lang, "invalid_weather")})
		return
	}
	if !byCoords {
		lat, lon = current.Coord.Lat, current.Coord.Lon
	}
	data := Weather{City: current.Name, Temperature: current.Main.Temp, FeelsLike: current.Main.FeelsLike, Status: current.Weather[0].Main, Description: current.Weather[0].Description, Humidity: current.Main.Humidity, WindSpeed: current.Wind.Speed, Icon: "https://openweathermap.org/img/wn/" + current.Weather[0].Icon + "@2x.png", Timezone: current.Timezone, Sunrise: current.Sys.Sunrise, Sunset: current.Sys.Sunset, ForecastSource: "3-hour"}
	var forecast forecastAPI
	forecastURL := fmt.Sprintf("https://api.openweathermap.org/data/2.5/forecast?lat=%f&lon=%f&units=metric&lang=%s&appid=%s", lat, lon, lang, url.QueryEscape(key))
	if err := fetchJSON(ctx, forecastURL, &forecast); err == nil {
		data.Hourly = forecastHours(forecast, current.Timezone)
		data.Forecast = dailyFrom3Hour(forecast, current.Timezone)
	} else {
		log.Printf("forecast fallback: %v", err)
	}
	if strings.EqualFold(os.Getenv("OPENWEATHER_ONECALL_ENABLED"), "true") {
		var oneCall oneCallAPI
		oneURL := fmt.Sprintf("https://api.openweathermap.org/data/3.0/onecall?lat=%f&lon=%f&units=metric&lang=%s&appid=%s", lat, lon, lang, url.QueryEscape(key))
		if err := fetchJSON(ctx, oneURL, &oneCall); err != nil {
			log.Printf("One Call data unavailable: %v", err)
		} else {
			uv := oneCall.Current.UVI
			data.UV = &uv
			if oneCall.Current.Sunrise > 0 {
				data.Sunrise = oneCall.Current.Sunrise
			}
			if oneCall.Current.Sunset > 0 {
				data.Sunset = oneCall.Current.Sunset
			}
			if len(oneCall.Hourly) > 0 {
				data.Hourly = hoursFromOneCall(oneCall)
			}
			if len(oneCall.Daily) > 0 {
				data.Forecast = dailyFromOneCall(oneCall, oneCall.TimezoneOffset)
			}
			for _, a := range oneCall.Alerts {
				data.OfficialAlerts = append(data.OfficialAlerts, WeatherAlert{Event: a.Event, Description: a.Description, Start: a.Start, End: a.End, Source: a.Sender})
			}
			data.ForecastSource = "hourly"
		}
	}
	data.Alerts = buildAdvisories(data, lang)
	storeCachedWeather(cacheKey, data)
	recordHistory(HistoryPoint{City: data.City, Time: time.Now().Unix(), Temperature: data.Temperature})
	writeJSON(w, 200, data)
}

func fetchJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("permintaan cuaca tidak dapat dibuat")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("layanan cuaca tidak dapat dihubungi")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		var payload struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&payload)
		return &upstreamError{status: resp.StatusCode, message: payload.Message}
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("respons layanan cuaca tidak valid")
	}
	return nil
}

func weatherLanguage(value string) string {
	if strings.EqualFold(value, "en") {
		return "en"
	}
	return "id"
}

func localized(lang, key string) string {
	id := map[string]string{
		"method": "Gunakan metode GET.", "city_required": "Parameter kota wajib diisi.", "coordinates_required": "Parameter lat dan lon harus diberikan bersama.",
		"missing_key": "OPENWEATHER_API_KEY belum diatur di environment backend.", "invalid_coordinates": "Koordinat lokasi tidak valid.", "invalid_weather": "Respons cuaca tidak memiliki kondisi.",
		"network": "Layanan cuaca tidak dapat dihubungi. Coba lagi sebentar.", "invalid_key": "Kunci OpenWeather tidak valid atau belum aktif. Periksa API key di konfigurasi backend.",
		"not_found": "Kota tidak ditemukan. Periksa ejaan nama kota lalu coba lagi.", "access": "Akses fitur ini belum aktif pada akun OpenWeather. Periksa paket API yang digunakan.",
		"quota": "Kuota OpenWeather sedang terlampaui. Coba lagi beberapa saat nanti.", "upstream": "Layanan OpenWeather sedang bermasalah. Coba lagi nanti.",
		"too_many": "Terlalu banyak permintaan. Tunggu sebentar sebelum mencoba lagi.", "rain_alert": "Peluang hujan tinggi", "rain_advice": "Prakiraan menunjukkan peluang hujan tinggi dalam beberapa jam ke depan. Pertimbangkan membawa payung.",
		"heat_alert": "Suhu tinggi", "heat_advice": "Cuaca sedang sangat panas. Minum cukup air dan batasi aktivitas berat di luar ruangan.",
	}
	en := map[string]string{
		"method": "Use the GET method.", "city_required": "A city name is required.", "coordinates_required": "Provide both lat and lon parameters.",
		"missing_key": "OPENWEATHER_API_KEY is not configured for the backend.", "invalid_coordinates": "The location coordinates are invalid.", "invalid_weather": "The weather response did not include conditions.",
		"network": "The weather service could not be reached. Please try again shortly.", "invalid_key": "The OpenWeather API key is invalid or inactive. Check the backend API key configuration.",
		"not_found": "City not found. Check the spelling and try again.", "access": "This feature is not enabled for your OpenWeather account. Check your API access plan.",
		"quota": "The OpenWeather request quota has been reached. Please try again later.", "upstream": "OpenWeather is temporarily unavailable. Please try again later.",
		"too_many": "Too many requests. Wait a moment before trying again.", "rain_alert": "High chance of rain", "rain_advice": "Forecasts show a high chance of rain in the next few hours. Consider bringing an umbrella.",
		"heat_alert": "High temperature", "heat_advice": "It is very hot. Drink enough water and limit strenuous outdoor activity.",
	}
	if lang == "en" {
		return en[key]
	}
	return id[key]
}

func publicWeatherError(err error, lang string) (int, string) {
	var upstream *upstreamError
	if !errors.As(err, &upstream) {
		return http.StatusBadGateway, localized(lang, "network")
	}
	switch upstream.status {
	case http.StatusUnauthorized:
		return http.StatusServiceUnavailable, localized(lang, "invalid_key")
	case http.StatusNotFound:
		return http.StatusNotFound, localized(lang, "not_found")
	case http.StatusForbidden:
		return http.StatusServiceUnavailable, localized(lang, "access")
	case http.StatusTooManyRequests:
		return http.StatusTooManyRequests, localized(lang, "quota")
	default:
		return http.StatusBadGateway, localized(lang, "upstream")
	}
}

func cacheDuration() time.Duration {
	seconds, err := strconv.Atoi(os.Getenv("WEATHER_CACHE_SECONDS"))
	if err != nil {
		seconds = 300
	}
	if seconds < 0 {
		seconds = 0
	}
	return time.Duration(seconds) * time.Second
}
func getCachedWeather(key string) (Weather, bool) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	entry, ok := weatherCache[key]
	if !ok {
		return Weather{}, false
	}
	if time.Now().After(entry.expires) {
		delete(weatherCache, key)
		return Weather{}, false
	}
	return entry.data, true
}
func storeCachedWeather(key string, data Weather) {
	duration := cacheDuration()
	if duration == 0 {
		return
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	weatherCache[key] = cachedWeather{data: data, expires: time.Now().Add(duration)}
}
func limitRequests(next http.Handler) http.Handler {
	limit := 120
	if configured, err := strconv.Atoi(os.Getenv("RATE_LIMIT_PER_MINUTE")); err == nil && configured > 0 {
		limit = configured
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		minute := time.Now().Unix() / 60
		rateMu.Lock()
		entry := rateWindows[host]
		if entry.minute != minute {
			entry = rateWindow{minute: minute}
		}
		entry.count++
		rateWindows[host] = entry
		if len(rateWindows) > 4096 {
			for ip, window := range rateWindows {
				if window.minute < minute-1 {
					delete(rateWindows, ip)
				}
			}
		}
		blocked := entry.count > limit
		rateMu.Unlock()
		if blocked {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": localized(weatherLanguage(r.URL.Query().Get("lang")), "too_many")})
			return
		}
		next.ServeHTTP(w, r)
	})
}
func forecastHours(f forecastAPI, _ int) []HourlyForecast {
	out := make([]HourlyForecast, 0, len(f.List))
	for _, v := range f.List {
		if len(v.Weather) == 0 {
			continue
		}
		out = append(out, HourlyForecast{Time: v.DT, Temperature: v.Main.Temp, Description: v.Weather[0].Description, Icon: "https://openweathermap.org/img/wn/" + v.Weather[0].Icon + ".png", Rain: v.Pop})
	}
	return out
}
func dailyFrom3Hour(f forecastAPI, offset int) []DailyForecast {
	type aggregate struct {
		item  DailyForecast
		count int
	}
	groups := map[string]*aggregate{}
	loc := time.FixedZone("city", offset)
	for _, v := range f.List {
		if len(v.Weather) == 0 {
			continue
		}
		date := time.Unix(v.DT, 0).In(loc).Format("2006-01-02")
		g := groups[date]
		if g == nil {
			g = &aggregate{item: DailyForecast{Date: date, Min: v.Main.Temp, Max: v.Main.Temp, Description: v.Weather[0].Description, Icon: "https://openweathermap.org/img/wn/" + v.Weather[0].Icon + "@2x.png"}}
			groups[date] = g
		}
		if v.Main.Temp < g.item.Min {
			g.item.Min = v.Main.Temp
		}
		if v.Main.Temp > g.item.Max {
			g.item.Max = v.Main.Temp
		}
		if v.Pop > g.item.Rain {
			g.item.Rain = v.Pop
		}
		g.count++
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := []DailyForecast{}
	for _, k := range keys {
		out = append(out, groups[k].item)
	}
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}
func hoursFromOneCall(data oneCallAPI) []HourlyForecast {
	out := []HourlyForecast{}
	for _, v := range data.Hourly {
		if len(v.Weather) == 0 {
			continue
		}
		out = append(out, HourlyForecast{Time: v.DT, Temperature: v.Temp, Description: v.Weather[0].Description, Icon: "https://openweathermap.org/img/wn/" + v.Weather[0].Icon + ".png", Rain: v.Pop})
	}
	return out
}
func dailyFromOneCall(data oneCallAPI, offset int) []DailyForecast {
	out := []DailyForecast{}
	for _, v := range data.Daily {
		if len(v.Weather) == 0 {
			continue
		}
		date := time.Unix(v.DT+int64(offset), 0).UTC().Format("2006-01-02")
		out = append(out, DailyForecast{Date: date, Min: v.Temp.Min, Max: v.Temp.Max, Description: v.Weather[0].Description, Icon: "https://openweathermap.org/img/wn/" + v.Weather[0].Icon + "@2x.png", Rain: v.Pop})
	}
	return out
}
func buildAdvisories(w Weather, lang string) []WeatherAlert {
	out := []WeatherAlert{}
	now := time.Now().Unix()
	for _, h := range w.Hourly {
		if h.Time < now || h.Time > now+12*3600 {
			continue
		}
		if h.Rain >= 0.7 {
			source := "Prakiraan"
			if lang == "en" {
				source = "Forecast"
			}
			out = append(out, WeatherAlert{Event: localized(lang, "rain_alert"), Description: localized(lang, "rain_advice"), Start: h.Time, End: h.Time + 3*3600, Source: source})
			break
		}
	}
	if w.Temperature >= 34 {
		out = append(out, WeatherAlert{Event: localized(lang, "heat_alert"), Description: localized(lang, "heat_advice"), Start: now, End: now + 3*3600, Source: "WeatherNow"})
	}
	return out
}
func historyPath() string {
	if p := strings.TrimSpace(os.Getenv("WEATHER_HISTORY_FILE")); p != "" {
		return p
	}
	return "weather_history.json"
}
func loadHistory() {
	b, e := os.ReadFile(historyPath())
	if e == nil {
		_ = json.Unmarshal(b, &history)
	}
}
func recordHistory(point HistoryPoint) {
	historyMu.Lock()
	defer historyMu.Unlock()
	history = append(history, point)
	if len(history) > 1000 {
		history = history[len(history)-1000:]
	}
	b, e := json.Marshal(history)
	if e == nil {
		if e = os.WriteFile(historyPath(), b, 0644); e != nil {
			log.Printf("gagal menyimpan riwayat cuaca: %v", e)
		}
	}
}
func getHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "Gunakan metode GET."})
		return
	}
	city := strings.TrimSpace(r.URL.Query().Get("city"))
	historyMu.Lock()
	defer historyMu.Unlock()
	out := []HistoryPoint{}
	for _, p := range history {
		if city == "" || strings.EqualFold(city, p.City) {
			out = append(out, p)
		}
	}
	if len(out) > 48 {
		out = out[len(out)-48:]
	}
	writeJSON(w, 200, out)
}
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
