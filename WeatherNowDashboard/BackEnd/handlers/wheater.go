package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"weathernowdashboard/models"
	"weathernowdashboard/utils"
)

func GetWeatherHandler(w http.ResponseWriter, r *http.Request) {
	city := r.URL.Query().Get("city")
	if city == "" {
		http.Error(w, "Missing city parameter", http.StatusBadRequest)
		return
	}

	apiKey := os.Getenv("API_KEY")
	url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?q=%s&units=metric&appid=%s", city, apiKey)

	body, err := utils.FetchAPI(url)
	if err != nil {
		http.Error(w, "Failed to fetch weather data", http.StatusInternalServerError)
		return
	}

	var weather models.WeatherResponse
	err = json.Unmarshal(body, &weather)
	if err != nil {
		http.Error(w, "Failed to parse weather data", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"city":        weather.Name,
		"temperature": weather.Main.Temp,
		"description": weather.Weather[0].Description,
		"humidity":    weather.Main.Humidity,
		"wind_speed":  weather.Wind.Speed,
		"icon":        weather.Weather[0].Icon,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
